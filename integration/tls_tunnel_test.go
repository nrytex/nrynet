package integration

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	clientagent "github.com/nrytex/nrynet/client/agent"
	"github.com/nrytex/nrynet/internal/agenttoken"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/tlspin"
	clienthub "github.com/nrytex/nrynet/server/client"
	"github.com/nrytex/nrynet/server/relay"
	serverTunnel "github.com/nrytex/nrynet/server/tunnel"
)

func TestTLSControlAndDataTunnelEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 2*time.Second)
	control := httptest.NewTLSServer(controlRouter(hub))
	defer control.Close()
	rawData := listenTCP(t)
	dataListener := tls.NewListener(rawData, &tls.Config{
		Certificates: control.TLS.Certificates, MinVersion: tls.VersionTLS13,
	})
	broker := relay.NewBroker(authService, store, 3*time.Second)
	runBroker(t, dataListener, broker)

	pinnedToken, err := agenttoken.WithCertificatePin(cleartext, tlspin.FromCertificate(control.Certificate()))
	if err != nil {
		t.Fatal(err)
	}
	agent := newTLSAgent(t, control.URL, dataListener.Addr().String(), pinnedToken)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "tls-device")
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

func newTLSAgent(t *testing.T, serverURL, dataAddress, token string) *clientagent.Agent {
	t.Helper()
	clientConfig := config.ClientConfig{
		ServerURL:   strings.Replace(serverURL, "https://", "wss://", 1) + "/agent/connect",
		DataAddress: dataAddress, Token: token, Name: "tls-agent", DeviceID: "tls-device",
	}
	client, err := clientagent.New(clientagent.NewOptions(config.Config{Client: clientConfig}, "test"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return client
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
