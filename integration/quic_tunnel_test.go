package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	clientagent "github.com/nat-link/nat-link/client/agent"
	netx "github.com/nat-link/nat-link/internal/advanced"
	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/server/advanced"
	clienthub "github.com/nat-link/nat-link/server/client"
	"github.com/nat-link/nat-link/server/relay"
	serverTunnel "github.com/nat-link/nat-link/server/tunnel"
)

func TestQUICTunnelEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 45*time.Second)
	broker := relay.NewBroker(authService, store, 3*time.Second)
	quicServer := startQUICServer(t, ctx, authService, hub, broker)

	echo := startEcho(t)
	defer echo.Close()
	agent := newQUICAgent(t, quicServer.Addr().String(), cleartext)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "quic-device")

	remotePort := reservePort(t)
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	tunnel := createTunnel(t, store, client.ID, echo.Addr().(*net.TCPAddr).Port, remotePort)
	if err := manager.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	assertEchoThroughPort(t, remotePort, []byte("quic-e2e"))
	assertConcurrentQUICTunnels(t, remotePort)
}

func assertConcurrentQUICTunnels(t *testing.T, remotePort int) {
	t.Helper()
	const connections = 8
	errors := make(chan error, connections)
	var wait sync.WaitGroup
	for index := 0; index < connections; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errors <- echoThroughPort(remotePort, []byte("quic-concurrent"))
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func startQUICServer(
	t *testing.T,
	ctx context.Context,
	authService *auth.Service,
	hub *clienthub.Hub,
	broker *relay.Broker,
) *advanced.QUICControlServer {
	t.Helper()
	cert, err := netx.SelfSignedCertificate()
	if err != nil {
		t.Fatal(err)
	}
	server, err := advanced.ListenQUIC(
		"127.0.0.1:0", netx.ServerTLSConfig(cert), authService, hub, broker,
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(ctx) }()
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func newQUICAgent(t *testing.T, quicAddress, token string) *clientagent.Agent {
	t.Helper()
	clientConfig := config.ClientConfig{
		ServerURL: "ws://127.0.0.1/unused", Transport: "quic", QUICAddress: quicAddress,
		Token: token, Name: "quic", DeviceID: "quic-device", InsecureSkipVerify: true,
	}
	client, err := clientagent.New(clientagent.NewOptions(config.Config{Client: clientConfig}, "test"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func assertEchoThroughPort(t *testing.T, remotePort int, want []byte) {
	t.Helper()
	if err := echoThroughPort(remotePort, want); err != nil {
		t.Fatal(err)
	}
}

func echoThroughPort(remotePort int, want []byte) error {
	visitor, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remotePort)), 2*time.Second)
	if err != nil {
		return err
	}
	defer visitor.Close()
	_ = visitor.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := visitor.Write(want); err != nil {
		return err
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(visitor, got); err != nil {
		return err
	}
	if string(got) != string(want) {
		return fmt.Errorf("relay response=%q want=%q", got, want)
	}
	return nil
}
