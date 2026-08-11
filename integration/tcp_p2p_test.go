package integration

import (
	"context"
	"io"
	"net"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	clienthub "github.com/nrytex/nrynet/server/client"
	"github.com/nrytex/nrynet/server/relay"
	serverTunnel "github.com/nrytex/nrynet/server/tunnel"
)

func TestTCPTunnelUsesP2PDirectPathWithRendezvous(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 2*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	defer control.Close()
	dataListener := listenTCP(t)
	broker := relay.NewBroker(authService, store, 3*time.Second)
	runBroker(t, dataListener, broker)
	echo := startEcho(t)
	defer echo.Close()
	agent := newAgent(t, control.URL, dataListener.Addr().String(), cleartext)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "integration-device")
	rdv := startRendezvous(t, ctx)
	remotePort := reservePort(t)
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	manager.SetRendezvousAddress(rdv.Addr().String())
	manager.SetP2PEnabled(true)
	tunnel := createTunnel(t, store, client.ID, echo.Addr().(*net.TCPAddr).Port, remotePort)
	if err := manager.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}

	visitor, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remotePort)), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer visitor.Close()
	_ = visitor.SetDeadline(time.Now().Add(15 * time.Second))
	want := []byte("tcp-p2p-e2e")
	if _, err := visitor.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(visitor, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("p2p response=%q want=%q", got, want)
	}
	assertEventRecorded(t, store, "p2p.tcp.direct")
}

func TestTCPTunnelFallsBackWhenP2PSetupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 10*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	defer control.Close()
	dataListener := listenTCP(t)
	broker := relay.NewBroker(authService, store, 3*time.Second)
	runBroker(t, dataListener, broker)
	echo := startEcho(t)
	defer echo.Close()
	agent := newAgent(t, control.URL, dataListener.Addr().String(), cleartext)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "integration-device")
	remotePort := reservePort(t)
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	manager.SetRendezvousAddress(net.JoinHostPort("127.0.0.1", strconv.Itoa(reserveUDPPort(t))))
	manager.SetP2PEnabled(true)
	tunnel := createTunnel(t, store, client.ID, echo.Addr().(*net.TCPAddr).Port, remotePort)
	if err := manager.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}

	visitor, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remotePort)), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer visitor.Close()
	_ = visitor.SetDeadline(time.Now().Add(10 * time.Second))
	want := []byte("tcp-p2p-fallback")
	if _, err := visitor.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(visitor, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("fallback response=%q want=%q", got, want)
	}
	assertEventRecorded(t, store, "p2p.tcp.fallback")
}

func TestP2PProtocolUsesDirectPathWithRendezvous(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 2*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	defer control.Close()
	dataListener := listenTCP(t)
	broker := relay.NewBroker(authService, store, 3*time.Second)
	runBroker(t, dataListener, broker)
	echo := startEcho(t)
	defer echo.Close()
	agent := newAgent(t, control.URL, dataListener.Addr().String(), cleartext)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "integration-device")
	rdv := startRendezvous(t, ctx)
	remotePort := reservePort(t)
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	manager.SetRendezvousAddress(rdv.Addr().String())
	manager.SetP2PEnabled(true)
	tunnel := createTunnelWithProtocol(t, store, client.ID, echo.Addr().(*net.TCPAddr).Port, remotePort, "p2p")
	if err := manager.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}

	visitor, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remotePort)), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer visitor.Close()
	_ = visitor.SetDeadline(time.Now().Add(15 * time.Second))
	want := []byte("p2p-protocol-e2e")
	if _, err := visitor.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(visitor, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("p2p protocol response=%q want=%q", got, want)
	}
	assertEventRecorded(t, store, "p2p.tcp.direct")
}
