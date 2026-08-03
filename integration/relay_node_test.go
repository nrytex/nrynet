package integration

import (
	"context"
	"io"
	"net"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/relay/runtime"
	serveradvanced "github.com/nrytex/nrynet/server/advanced"
	clienthub "github.com/nrytex/nrynet/server/client"
	"github.com/nrytex/nrynet/server/relay"
	serverTunnel "github.com/nrytex/nrynet/server/tunnel"
)

func TestRelayNodeReassignsRealVisitorStreams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 20*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	defer control.Close()
	broker := relay.NewBroker(authService, store, 3*time.Second)
	data := listenTCP(t)
	runBroker(t, data, broker)
	agent := newAgent(t, control.URL, data.Addr().String(), cleartext)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "integration-device")
	echo := startEcho(t)
	defer echo.Close()
	registry := netx.NewRelayRegistry(30 * time.Second)
	manager := serverTunnel.NewManager(store, hub, broker)
	manager.SetRelayRegistry(registry, &serveradvanced.RemoteRelayNode{
		Token: "relay-secret", BrokerAddress: data.Addr().String(), Registry: registry,
	})
	broker.SetRelayVisitorHandler("relay-secret", manager.RouteRelayVisitor)
	defer manager.Close()
	remotePort := reservePort(t)
	tunnel := createTunnel(t, store, client.ID, echo.Addr().(*net.TCPAddr).Port, remotePort)
	if err := manager.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	assertEchoThroughPort(t, remotePort, []byte("central-fallback"))
	waitForNoRelayConnections(t, manager)
	startRelayNode(t, "relay-a", "127.0.0.2", registry, data.Addr().String())
	startRelayNode(t, "relay-b", "127.0.0.3", registry, data.Addr().String())
	manager.AssignAvailableRelayTunnels(ctx)
	assertEchoThroughAddress(t, net.JoinHostPort("127.0.0.2", strconv.Itoa(remotePort)), []byte("relay-a"))
	waitForNoRelayConnections(t, manager)
	registry.MarkUnhealthy("relay-a")
	if err := manager.ReassignRelayTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	assertEchoThroughAddress(t, net.JoinHostPort("127.0.0.3", strconv.Itoa(remotePort)), []byte("relay-b"))
}

func waitForNoRelayConnections(t *testing.T, manager *serverTunnel.Manager) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if manager.ActiveConnections() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("previous relay connection did not close")
}

func startRelayNode(t *testing.T, id, bindHost string, registry *netx.RelayRegistry, broker string) {
	t.Helper()
	control := listenTCP(t)
	node, err := runtime.New(runtime.Config{
		ID: id, Address: bindHost, ControlAddress: "http://" + control.Addr().String(),
		BindHost: bindHost, BrokerAddress: broker, Token: "relay-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = node.ServeControl(control) }()
	t.Cleanup(func() { node.Close(); _ = control.Close() })
	if _, err := registry.Register(netx.RelayNode{
		ID: id, Address: bindHost, ControlAddr: "http://" + control.Addr().String(),
	}); err != nil {
		t.Fatal(err)
	}
}

func assertEchoThroughAddress(t *testing.T, address string, want []byte) {
	t.Helper()
	visitor, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer visitor.Close()
	_ = visitor.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := visitor.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(visitor, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("relay response=%q want=%q", got, want)
	}
}
