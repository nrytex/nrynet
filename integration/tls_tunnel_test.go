package integration

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	clientagent "github.com/nat-link/nat-link/client/agent"
	"github.com/nat-link/nat-link/internal/config"
	clienthub "github.com/nat-link/nat-link/server/client"
	"github.com/nat-link/nat-link/server/relay"
	serverTunnel "github.com/nat-link/nat-link/server/tunnel"
)

func TestTLSControlAndDataTunnelEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 2*time.Second)
	control := httptest.NewTLSServer(controlRouter(hub))
	defer control.Close()
	rawData := listenTCP(t)
	dataListener := tls.NewListener(rawData, &tls.Config{
		Certificates: control.TLS.Certificates, MinVersion: tls.VersionTLS13,
	})
	defer dataListener.Close()
	broker := relay.NewBroker(authService, store, 3*time.Second)
	go func() { _ = broker.Run(dataListener) }()

	caFile := writeTestCA(t, control.Certificate().Raw)
	agent := newTLSAgent(t, control.URL, dataListener.Addr().String(), cleartext, caFile)
	go func() { _ = agent.Run(ctx) }()
	client := waitForClient(t, store, "tls-device")
	echo := startEcho(t)
	defer echo.Close()
	remotePort := reservePort(t)
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	tunnel := createTunnel(t, store, client.ID, echo.Addr().(*net.TCPAddr).Port, remotePort)
	if err := manager.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	assertTCPPayload(t, remotePort)
}

func newTLSAgent(t *testing.T, serverURL, dataAddress, token, caFile string) *clientagent.Agent {
	t.Helper()
	clientConfig := config.ClientConfig{
		ServerURL:   strings.Replace(serverURL, "https://", "wss://", 1) + "/agent/connect",
		DataAddress: dataAddress, Token: token, Name: "tls-agent", DeviceID: "tls-device",
		CAFile: caFile,
	}
	client, err := clientagent.New(clientagent.NewOptions(config.Config{Client: clientConfig}, "test"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeTestCA(t *testing.T, certificate []byte) string {
	t.Helper()
	path := t.TempDir() + "/ca.pem"
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertTCPPayload(t *testing.T, remotePort int) {
	t.Helper()
	visitor, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remotePort)), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer visitor.Close()
	_ = visitor.SetDeadline(time.Now().Add(5 * time.Second))
	want := []byte("encrypted-control-and-data")
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
