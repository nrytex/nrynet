package integration

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	clientagent "github.com/nat-link/nat-link/client/agent"
	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/storage"
	clienthub "github.com/nat-link/nat-link/server/client"
	"github.com/nat-link/nat-link/server/relay"
	serverTunnel "github.com/nat-link/nat-link/server/tunnel"
)

func TestTCPTunnelEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 2*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	defer control.Close()
	dataListener := listenTCP(t)
	defer dataListener.Close()
	broker := relay.NewBroker(authService, store, 3*time.Second)
	go func() { _ = broker.Run(dataListener) }()

	echo := startEcho(t)
	defer echo.Close()
	agent := newAgent(t, control.URL, dataListener.Addr().String(), cleartext)
	go func() { _ = agent.Run(ctx) }()
	client := waitForClient(t, store, "integration-device")

	remotePort := reservePort(t)
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	tunnel := createTunnel(t, store, client.ID, echo.Addr().(*net.TCPAddr).Port, remotePort)
	if err := manager.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}

	visitor, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remotePort)), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer visitor.Close()
	_ = visitor.SetDeadline(time.Now().Add(5 * time.Second))
	want := []byte("nat-link-e2e")
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

func testServices(t *testing.T) (*storage.Store, *auth.Service) {
	t.Helper()
	store, err := storage.Open(t.TempDir() + "/integration.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := auth.New(context.Background(), store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return store, service
}

func createToken(t *testing.T, ctx context.Context, service *auth.Service) string {
	t.Helper()
	_, value, err := service.CreateAgentToken(ctx, "integration")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func controlRouter(hub *clienthub.Hub) *gin.Engine {
	router := gin.New()
	router.GET("/agent/connect", hub.Handle)
	return router
}

func newAgent(t *testing.T, serverURL, dataAddress, token string) *clientagent.Agent {
	t.Helper()
	clientConfig := config.ClientConfig{
		ServerURL:   strings.Replace(serverURL, "http://", "ws://", 1) + "/agent/connect",
		DataAddress: dataAddress, Token: token, Name: "integration", DeviceID: "integration-device",
	}
	client, err := clientagent.New(clientagent.NewOptions(config.Config{Client: clientConfig}, "test"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func waitForClient(t *testing.T, store *storage.Store, deviceID string) model.Client {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		client, err := store.GetClientByDevice(context.Background(), deviceID)
		if err == nil && client.Status == "online" {
			return client
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("agent did not connect")
	return model.Client{}
}

func createTunnel(t *testing.T, store *storage.Store, clientID string, localPort, remotePort int) model.Tunnel {
	t.Helper()
	tunnel, err := store.CreateTunnel(context.Background(), model.Tunnel{
		Name: "echo", Protocol: "tcp", ClientID: clientID,
		LocalHost: "127.0.0.1", LocalPort: localPort, RemotePort: remotePort,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tunnel
}

func startEcho(t *testing.T) net.Listener {
	t.Helper()
	listener := listenTCP(t)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(conn, conn); _ = conn.Close() }()
		}
	}()
	return listener
}

func listenTCP(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func reservePort(t *testing.T) int {
	t.Helper()
	listener := listenTCP(t)
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}
