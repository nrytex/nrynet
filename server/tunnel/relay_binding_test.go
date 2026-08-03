package tunnel

import (
	"context"
	"net"
	"testing"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/storage"
)

func TestRelayVisitorAllowlistUsesOriginalAddress(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(t.TempDir() + "/relay-allowlist.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.DB().ExecContext(ctx, `INSERT INTO tokens
		(id, name, prefix, secret_hash, disabled, created_at) VALUES('token', 'test', 'prefix', 'hash', 0, ?)`, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(ctx, "token", storage.ClientHello{Name: "client", DeviceID: "device"})
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := store.CreateTunnel(ctx, model.Tunnel{
		Name: "restricted", ClientID: client.ID, Protocol: "tcp",
		LocalHost: "127.0.0.1", LocalPort: 1, RemotePort: 2,
		IPAllowlist: []string{"203.0.113.7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTunnelStatus(ctx, tunnel.ID, "running"); err != nil {
		t.Fatal(err)
	}
	registry := netx.NewRelayRegistry(time.Minute)
	if _, err := registry.Register(netx.RelayNode{ID: "relay", Address: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AssignTunnel(tunnel.ID); err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	visitor := testRemoteConn{Conn: left, remote: &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 1234}}
	manager := &Manager{store: store, registry: registry}
	if err := manager.RouteRelayVisitor("relay", tunnel.ID, "198.51.100.9:4567", visitor); err == nil {
		t.Fatal("visitor outside the allowlist was accepted through relay")
	}
}

type testRemoteConn struct {
	net.Conn
	remote net.Addr
}

func (c testRemoteConn) RemoteAddr() net.Addr { return c.remote }
