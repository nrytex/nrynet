package integration

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	clientagent "github.com/nrytex/nrynet/client/agent"
	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/agenttoken"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/tlspin"
	clienthub "github.com/nrytex/nrynet/server/client"
	"github.com/nrytex/nrynet/server/relay"
	serverTunnel "github.com/nrytex/nrynet/server/tunnel"
)

func TestQUICControlFallsBackToWebSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 45*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	defer control.Close()
	dataListener := listenTCP(t)
	broker := relay.NewBroker(authService, store, 3*time.Second)
	runBroker(t, dataListener, broker)
	cert, err := netx.SelfSignedCertificate()
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	quicServer, err := netx.ListenQUIC("127.0.0.1:0", netx.ServerTLSConfig(cert), func(context.Context, netx.AuthRequest, net.Addr) error {
		return errors.New("intentionally unavailable QUIC control")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer quicServer.Close()
	go func() {
		for ctx.Err() == nil {
			_, err := quicServer.Accept(ctx)
			if err != nil && ctx.Err() != nil {
				return
			}
		}
	}()
	pinnedToken, err := agenttoken.WithCertificatePin(cleartext, tlspin.FromCertificate(certificate))
	if err != nil {
		t.Fatal(err)
	}
	agentConfig := config.ClientConfig{
		ServerURL:   strings.Replace(control.URL, "http://", "ws://", 1) + "/agent/connect",
		DataAddress: dataListener.Addr().String(),
		Transport:   "quic", QUICAddress: quicServer.Addr().String(),
		Token: pinnedToken, Name: "quic-fallback", DeviceID: "quic-fallback-device",
	}
	agent, err := clientagent.New(clientagent.NewOptions(config.Config{Client: agentConfig}, "test"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	runAgent(t, ctx, cancel, agent)
	waitForClient(t, store, hub, "quic-fallback-device")
	echo := startEcho(t)
	defer echo.Close()
	remotePort := reservePort(t)
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	client, err := store.GetClientByDevice(ctx, "quic-fallback-device")
	if err != nil {
		t.Fatal(err)
	}
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
	want := []byte("quic-fallback-e2e")
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
