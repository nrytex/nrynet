package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/model"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "security.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func insertTestToken(t *testing.T, store *Store, id string) string {
	t.Helper()
	_, err := store.DB().Exec(`INSERT INTO tokens
        (id, name, prefix, secret_hash, disabled, created_at) VALUES(?, ?, ?, ?, 0, ?)`,
		id, id, id, "hash-"+id, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestClientDeviceCannotBeReboundToAnotherToken(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	first := insertTestToken(t, store, "first")
	second := insertTestToken(t, store, "second")
	hello := ClientHello{Name: "device", DeviceID: "stable-device", IP: "127.0.0.1"}
	client, err := store.UpsertClient(ctx, first, hello)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertClient(ctx, second, hello); err == nil {
		t.Fatal("device was rebound to a different token")
	}
	stored, err := store.GetClient(ctx, client.ID)
	if err != nil || stored.TokenID != first {
		t.Fatalf("client token changed: client=%+v err=%v", stored, err)
	}
}

func TestDeleteTokenRemovesBoundClientsAndTunnels(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tokenID := insertTestToken(t, store, "delete")
	client, err := store.UpsertClient(ctx, tokenID, ClientHello{Name: "device", DeviceID: "delete-device"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTunnel(ctx, model.Tunnel{
		Name: "delete", ClientID: client.ID, Protocol: "tcp",
		LocalHost: "127.0.0.1", LocalPort: 80, RemotePort: 6080,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteToken(ctx, tokenID); err != nil {
		t.Fatal(err)
	}
	if clients, _ := store.ListClients(ctx); len(clients) != 0 {
		t.Fatalf("bound clients remain: %+v", clients)
	}
	if tunnels, _ := store.ListTunnels(ctx); len(tunnels) != 0 {
		t.Fatalf("bound tunnels remain: %+v", tunnels)
	}
}

func TestDeleteClientRevokesOnlyItsDevice(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tokenID := insertTestToken(t, store, "shared")
	firstHello := ClientHello{Name: "first", DeviceID: "first-device"}
	first, err := store.UpsertClient(ctx, tokenID, firstHello)
	if err != nil {
		t.Fatal(err)
	}
	secondHello := ClientHello{Name: "second", DeviceID: "second-device"}
	if _, err := store.UpsertClient(ctx, tokenID, secondHello); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeClientDevice(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertClient(ctx, tokenID, firstHello); err == nil {
		t.Fatal("revoked device reconnected before deletion completed")
	}
	if err := store.DeleteClient(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertClient(ctx, tokenID, firstHello); err == nil {
		t.Fatal("deleted device identity was recreated")
	}
	if _, err := store.UpsertClient(ctx, tokenID, secondHello); err != nil {
		t.Fatalf("other device using shared token was revoked: %v", err)
	}
}
