package storage

import (
	"context"
	"testing"
	"time"

	"github.com/nat-link/nat-link/internal/model"
)

func TestTrafficAndEventManagement(t *testing.T) {
	ctx := context.Background()
	store, tunnel := analyticsStore(t)
	if err := store.RecordTraffic(ctx, tunnel.ID, 100, 250); err != nil {
		t.Fatal(err)
	}
	summary, err := store.TrafficSummary(ctx, time.Now().Add(-time.Hour))
	if err != nil || summary.Upload != 100 || summary.Download != 250 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	byTunnel, err := store.TrafficByTunnel(ctx, time.Now().Add(-time.Hour))
	if err != nil || len(byTunnel) != 1 || byTunnel[0].Upload != 100 {
		t.Fatalf("tunnel traffic=%+v err=%v", byTunnel, err)
	}
	byClient, err := store.TrafficByClient(ctx, time.Now().Add(-time.Hour))
	if err != nil || len(byClient) != 1 || byClient[0].Download != 250 {
		t.Fatalf("client traffic=%+v err=%v", byClient, err)
	}
	if err := store.RecordEvent(ctx, "info", "test.event", "searchable message", map[string]any{"id": 1}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListEvents(ctx, EventFilter{Keyword: "searchable"})
	if err != nil || len(events) != 1 || events[0].Event != "test.event" {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	deleted, err := store.ClearEvents(ctx, time.Now().Add(time.Second))
	if err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
}

func TestSQLitePersistsConfigurationAndMetricsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/restart.db"
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB().ExecContext(ctx, `INSERT INTO tokens
		(id, name, prefix, secret_hash, disabled, created_at) VALUES('token', 'test', 'prefix', 'hash', 0, ?)`, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(ctx, "token", ClientHello{Name: "client", DeviceID: "restart-device"})
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := store.CreateTunnel(ctx, model.Tunnel{
		Name: "persistent", ClientID: client.ID, Protocol: "tcp",
		LocalHost: "127.0.0.1", LocalPort: 80, RemotePort: 6001,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordTraffic(ctx, tunnel.ID, 123, 456); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.GetTunnel(ctx, tunnel.ID)
	if err != nil || persisted.Name != tunnel.Name || persisted.ClientID != client.ID {
		t.Fatalf("persisted tunnel=%+v err=%v", persisted, err)
	}
	summary, err := reopened.TrafficSummary(ctx, time.Now().Add(-time.Hour))
	if err != nil || summary.Upload != 123 || summary.Download != 456 {
		t.Fatalf("persisted metrics=%+v err=%v", summary, err)
	}
}

func analyticsStore(t *testing.T) (*Store, model.Tunnel) {
	t.Helper()
	store, err := Open(t.TempDir() + "/analytics.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	_, err = store.DB().ExecContext(ctx, `INSERT INTO tokens
        (id, name, prefix, secret_hash, disabled, created_at) VALUES('token', 'test', 'prefix', 'hash', 0, ?)`, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(ctx, "token", ClientHello{Name: "client", DeviceID: "device", IP: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := store.CreateTunnel(ctx, model.Tunnel{
		Name: "test", ClientID: client.ID, Protocol: "tcp",
		LocalHost: "127.0.0.1", LocalPort: 80, RemotePort: 6000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, tunnel
}
