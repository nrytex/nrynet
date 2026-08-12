package integration

import (
	"context"
	"log/slog"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	clientagent "github.com/nrytex/nrynet/client/agent"
	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/storage"
	clienthub "github.com/nrytex/nrynet/server/client"
	"github.com/nrytex/nrynet/server/relay"
	serverTunnel "github.com/nrytex/nrynet/server/tunnel"
)

func TestUDPTunnelEndToEnd(t *testing.T) {
	env := startUDPIntegration(t)
	echo := startUDPEcho(t)
	defer echo.Close()
	remotePort := reserveUDPPort(t)
	tunnel := createUDPTunnel(t, env.store, env.client.ID, echo.LocalAddr().(*net.UDPAddr).Port, remotePort)
	if err := env.manager.StartTunnel(env.ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	visitor := dialUDPVisitor(t, remotePort)
	defer visitor.Close()
	assertUDPEcho(t, visitor, "first-packet")
	assertUDPEcho(t, visitor, "second-packet")
	waitForTrafficDirections(t, env.store)
	assertEventRecorded(t, env.store, "p2p.fallback")
}

func TestUDPTunnelUsesP2PDirectPathWithRendezvous(t *testing.T) {
	env := startUDPIntegration(t)
	rdv := startRendezvous(t, env.ctx)
	env.manager.SetRendezvousAddress(rdv.Addr().String())
	echo := startUDPEcho(t)
	defer echo.Close()
	remotePort := reserveUDPPort(t)
	tunnel := createUDPTunnel(t, env.store, env.client.ID, echo.LocalAddr().(*net.UDPAddr).Port, remotePort)
	if err := env.manager.StartTunnel(env.ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	visitor := dialUDPVisitor(t, remotePort)
	defer visitor.Close()
	assertUDPEcho(t, visitor, "p2p-direct")
	assertUDPEcho(t, visitor, "p2p-direct-reused")
	assertEventRecorded(t, env.store, "p2p.direct")
}

func TestUDPP2PDelayedResponseDoesNotDuplicateRequest(t *testing.T) {
	env := startUDPIntegration(t)
	rdv := startRendezvous(t, env.ctx)
	env.manager.SetRendezvousAddress(rdv.Addr().String())
	echo, requests := startDelayedUDPEcho(t, 1500*time.Millisecond)
	defer echo.Close()
	remotePort := reserveUDPPort(t)
	tunnel := createUDPTunnel(t, env.store, env.client.ID, echo.LocalAddr().(*net.UDPAddr).Port, remotePort)
	if err := env.manager.StartTunnel(env.ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	visitor := dialUDPVisitor(t, remotePort)
	defer visitor.Close()
	assertUDPEcho(t, visitor, "slow-direct")
	time.Sleep(200 * time.Millisecond)
	if count := requests.Load(); count != 1 {
		t.Fatalf("local UDP service received %d copies of one visitor request", count)
	}
}

func TestUDPTunnelRejectsIPAllowlist(t *testing.T) {
	env := startUDPIntegration(t)
	echo := startUDPEcho(t)
	defer echo.Close()
	remotePort := reserveUDPPort(t)
	tunnel := createDeniedUDPTunnel(t, env.store, env.client.ID, echo.LocalAddr().(*net.UDPAddr).Port, remotePort)
	if err := env.manager.StartTunnel(env.ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	visitor := dialUDPVisitor(t, remotePort)
	defer visitor.Close()
	_ = visitor.SetDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err := visitor.Write([]byte("blocked")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 32)
	if n, err := visitor.Read(buffer); err == nil {
		t.Fatalf("unexpected udp response %q", string(buffer[:n]))
	}
}

type udpIntegration struct {
	ctx     context.Context
	store   *storage.Store
	client  model.Client
	manager *serverTunnel.Manager
}

func startUDPIntegration(t *testing.T) udpIntegration {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 2*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	t.Cleanup(control.Close)
	dataListener := listenTCP(t)
	broker := relay.NewBroker(authService, store, 3*time.Second)
	runBroker(t, dataListener, broker)
	agent := newUDPAgent(t, control.URL, dataListener.Addr().String(), cleartext)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "udp-integration-device")
	manager := serverTunnel.NewManager(store, hub, broker)
	t.Cleanup(func() { _ = manager.Close() })
	return udpIntegration{ctx: ctx, store: store, client: client, manager: manager}
}

func newUDPAgent(t *testing.T, serverURL, dataAddress, token string) *clientagent.Agent {
	t.Helper()
	clientConfig := config.ClientConfig{
		ServerURL:   strings.Replace(serverURL, "http://", "ws://", 1) + "/agent/connect",
		DataAddress: dataAddress, Token: token, Name: "udp-integration", DeviceID: "udp-integration-device",
	}
	client, err := clientagent.New(clientagent.NewOptions(config.Config{Client: clientConfig}, "test"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func createUDPTunnel(t *testing.T, store *storage.Store, clientID string, localPort, remotePort int) model.Tunnel {
	t.Helper()
	tunnel, err := store.CreateTunnel(context.Background(), model.Tunnel{
		Name: "udp-echo", Protocol: "udp", ClientID: clientID, LocalHost: "127.0.0.1",
		LocalPort: localPort, RemotePort: remotePort, IPAllowlist: []string{"127.0.0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tunnel
}

func createDeniedUDPTunnel(t *testing.T, store *storage.Store, clientID string, localPort, remotePort int) model.Tunnel {
	t.Helper()
	tunnel, err := store.CreateTunnel(context.Background(), model.Tunnel{
		Name: "udp-denied", Protocol: "udp", ClientID: clientID, LocalHost: "127.0.0.1",
		LocalPort: localPort, RemotePort: remotePort, IPAllowlist: []string{"192.0.2.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tunnel
}

func startUDPEcho(t *testing.T) *net.UDPConn {
	t.Helper()
	conn := listenUDP(t)
	go func() {
		buffer := make([]byte, 64*1024)
		for {
			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			_, _ = conn.WriteToUDP(buffer[:n], addr)
		}
	}()
	return conn
}

func startDelayedUDPEcho(t *testing.T, delay time.Duration) (*net.UDPConn, *atomic.Int32) {
	t.Helper()
	conn := listenUDP(t)
	requests := &atomic.Int32{}
	go func() {
		buffer := make([]byte, 64*1024)
		for {
			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			requests.Add(1)
			payload := append([]byte(nil), buffer[:n]...)
			time.Sleep(delay)
			_, _ = conn.WriteToUDP(payload, addr)
		}
	}()
	return conn, requests
}

func dialUDPVisitor(t *testing.T, remotePort int) *net.UDPConn {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remotePort)))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func assertUDPEcho(t *testing.T, conn *net.UDPConn, text string) {
	t.Helper()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 64*1024)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if string(buffer[:n]) != text {
		t.Fatalf("udp response=%q want=%q", string(buffer[:n]), text)
	}
}

func reserveUDPPort(t *testing.T) int {
	t.Helper()
	conn := listenUDP(t)
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()
	return port
}

func listenUDP(t *testing.T) *net.UDPConn {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func startRendezvous(t *testing.T, ctx context.Context) *netx.RendezvousServer {
	t.Helper()
	server, err := netx.ListenRendezvous("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(ctx) }()
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func assertEventRecorded(t *testing.T, store *storage.Store, eventName string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, err := store.ListEvents(context.Background(), storage.EventFilter{Keyword: eventName})
		if err == nil && len(events) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("event %s was not recorded", eventName)
}

func assertNoEventRecorded(t *testing.T, store *storage.Store, eventName string) {
	t.Helper()
	events, err := store.ListEvents(context.Background(), storage.EventFilter{Keyword: eventName})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("unexpected event %s: %+v", eventName, events)
	}
}
