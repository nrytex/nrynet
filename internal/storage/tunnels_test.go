package storage

import (
	"context"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/model"
)

func TestCreateTunnelAcceptsP2PProtocol(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/p2p.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.DB().ExecContext(ctx, `INSERT INTO tokens
		(id, name, prefix, secret_hash, disabled, created_at) VALUES('token', 'test', 'prefix', 'hash', 0, ?)`, time.Now()); err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(ctx, "token", ClientHello{Name: "client", DeviceID: "p2p-device"})
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := store.CreateTunnel(ctx, model.Tunnel{
		Name: "direct", ClientID: client.ID, Protocol: "p2p",
		LocalHost: "127.0.0.1", LocalPort: 8080, RemotePort: 6000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tunnel.Protocol != "p2p" {
		t.Fatalf("protocol=%q, want p2p", tunnel.Protocol)
	}
	if tunnel.VisitorToken != "" {
		t.Fatalf("p2p tunnel unexpectedly received visitor token")
	}

	visitor, err := store.CreateTunnel(ctx, model.Tunnel{
		Name: "browser", ClientID: client.ID, Protocol: "visitor_webrtc",
		LocalHost: "127.0.0.1", LocalPort: 8081,
	})
	if err != nil {
		t.Fatal(err)
	}
	if visitor.VisitorToken == "" {
		t.Fatal("visitor_webrtc tunnel did not receive a token")
	}
	originalToken := visitor.VisitorToken
	visitor.Name = "browser-updated"
	visitor.VisitorToken = ""
	updated, err := store.UpdateTunnel(ctx, visitor)
	if err != nil {
		t.Fatal(err)
	}
	if updated.VisitorToken == "" || updated.VisitorToken != originalToken {
		t.Fatalf("visitor token was not preserved: updated=%q original=%q", updated.VisitorToken, originalToken)
	}
}
