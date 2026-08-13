package tunnel

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nrytex/nrynet/internal/storage"
)

func TestTrafficRecorderBatchesDeltas(t *testing.T) {
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	recorder := newTrafficRecorder(store)
	clientID := uuid.NewString()
	tunnelID := uuid.NewString()
	now := time.Now().UTC()
	tokenID := uuid.NewString()
	if _, err := store.DB().ExecContext(context.Background(), `INSERT INTO tokens
		(id, name, prefix, secret_hash, created_at) VALUES(?, ?, ?, ?, ?)`,
		tokenID, "test-2", "test-2", "hash", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(context.Background(), `INSERT INTO clients
		(id, name, device_id, token_id, status, ip, os, version, last_online, created_at)
		VALUES(?, ?, ?, ?, 'online', '', '', '', ?, ?)`, clientID, "client", "device", tokenID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(context.Background(), `INSERT INTO tunnels
		(id, client_id, name, protocol, local_host, local_port, remote_port, status, created_at, updated_at)
		VALUES(?, ?, ?, 'tcp', '127.0.0.1', 8080, 18080, 'running', ?, ?)`, tunnelID, clientID, "traffic", now, now); err != nil {
		t.Fatal(err)
	}
	recorder.add(tunnelID, 10, 20)
	recorder.add(tunnelID, 30, 40)
	recorder.close()
	traffic, err := store.TrafficForClient(context.Background(), clientID, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if traffic.Upload != 40 || traffic.Download != 60 {
		t.Fatalf("traffic=%+v want upload=40 download=60", traffic)
	}
}

func TestRecordTrafficBatchWritesOneTransaction(t *testing.T) {
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	clientID := uuid.NewString()
	tunnelID := uuid.NewString()
	now := time.Now().UTC()
	tokenID := uuid.NewString()
	if _, err := store.DB().ExecContext(context.Background(), `INSERT INTO tokens
		(id, name, prefix, secret_hash, created_at) VALUES(?, ?, ?, ?, ?)`,
		tokenID, "batch", "batch", "hash", now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(context.Background(), `INSERT INTO clients
		(id, name, device_id, token_id, status, ip, os, version, last_online, created_at)
		VALUES(?, ?, ?, ?, 'online', '', '', '', ?, ?)`, clientID, "client", "device", tokenID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(context.Background(), `INSERT INTO tunnels
		(id, client_id, name, protocol, local_host, local_port, remote_port, status, created_at, updated_at)
		VALUES(?, ?, ?, 'tcp', '127.0.0.1', 8080, 18080, 'running', ?, ?)`, tunnelID, clientID, "batch", now, now); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordTrafficBatch(context.Background(), []storage.TrafficDelta{
		{TunnelID: tunnelID, Upload: 3, Download: 4},
		{TunnelID: tunnelID, Upload: 5, Download: 6},
	}); err != nil {
		t.Fatal(err)
	}
	traffic, err := store.TrafficForClient(context.Background(), clientID, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if traffic.Upload != 8 || traffic.Download != 10 {
		t.Fatalf("traffic=%+v want upload=8 download=10", traffic)
	}
}
