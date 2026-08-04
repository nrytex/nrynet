package app

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	clientagent "github.com/nrytex/nrynet/client/agent"
	"github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/agenttoken"
	"github.com/nrytex/nrynet/internal/auth"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/tlspin"
)

func TestAppServesWSPlainDataAndWSSTLSDataTogether(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	certFile, keyFile, pin := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = application.Run() }()
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	waitPlainHealth(t, "http://"+cfg.Server.PlainListen)
	waitTLSHealth(t, "https://"+cfg.Server.Listen)

	authService, err := auth.New(ctx, application.store, cfg.Server.JWTTTL)
	if err != nil {
		t.Fatal(err)
	}
	_, clearToken, err := authService.CreateAgentToken(ctx, "dual-transport")
	if err != nil {
		t.Fatal(err)
	}
	pinnedToken, err := agenttoken.WithCertificatePin(clearToken, pin)
	if err != nil {
		t.Fatal(err)
	}
	plainAgent := newAppAgent(t, "ws://"+cfg.Server.PlainListen+"/agent/connect",
		cfg.Server.PlainDataListen, clearToken, "plain-device")
	tlsAgent := newAppAgent(t, "wss://"+cfg.Server.Listen+"/agent/connect",
		cfg.Server.DataListen, pinnedToken, "tls-device")
	runTestAgent(t, ctx, plainAgent)
	runTestAgent(t, ctx, tlsAgent)

	plainClient := waitForAppClient(t, application, "plain-device")
	tlsClient := waitForAppClient(t, application, "tls-device")
	assertAppTunnel(t, ctx, application, plainClient.ID, "plain-data")
	assertAppTunnel(t, ctx, application, tlsClient.ID, "tls-data")
}

func TestPlainControlBindFailureStopsStartup(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	cfg.Server.PlainListen = occupied.Addr().String()
	application, _, err := New(context.Background(), cfg)
	if application != nil {
		_ = application.Shutdown(context.Background())
	}
	if err == nil {
		t.Fatal("plain control bind conflict was accepted")
	}
}

func TestShutdownReleasesPlaintextPorts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = application.Run() }()
	waitPlainHealth(t, "http://"+cfg.Server.PlainListen)
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCanListen(t, "tcp", cfg.Server.PlainListen)
	assertCanListen(t, "tcp", cfg.Server.PlainDataListen)
}

func dualTransportConfig(t *testing.T, certFile, keyFile string) config.Config {
	t.Helper()
	return config.Config{Server: config.ServerConfig{
		Listen: RendezvousFreeTCP(t), PlainEnabled: true, PlainListen: RendezvousFreeTCP(t),
		DataListen: RendezvousFreeTCP(t), PlainDataListen: RendezvousFreeTCP(t),
		HTTPListen: RendezvousFreeTCP(t), QUICListen: RendezvousFreeUDP(t),
		RendezvousListen: RendezvousFreeUDP(t), Database: filepath.Join(t.TempDir(), "app.db"),
		JWTTTL: time.Hour, JWTTTLText: "1h", HeartbeatTimeout: 2 * time.Second,
		HeartbeatText: "2s", TLS: config.TLSConfig{Enabled: true, CertFile: certFile, KeyFile: keyFile},
		Bootstrap: config.BootstrapConfig{AdminUsername: "admin", AdminPassword: "password-1234"},
	}}
}

func assertCanListen(t *testing.T, network, address string) {
	t.Helper()
	listener, err := net.Listen(network, address)
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()
}

func newAppAgent(t *testing.T, serverURL, dataAddress, token, deviceID string) *clientagent.Agent {
	t.Helper()
	clientConfig := config.ClientConfig{
		ServerURL: serverURL, DataAddress: dataAddress, Token: token,
		Name: deviceID, DeviceID: deviceID, Transport: "websocket",
	}
	agent, err := clientagent.New(clientagent.NewOptions(config.Config{Client: clientConfig}, "test"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

func runTestAgent(t *testing.T, ctx context.Context, agent *clientagent.Agent) {
	t.Helper()
	go func() { _ = agent.Run(ctx) }()
}

func waitForAppClient(t *testing.T, application *App, deviceID string) model.Client {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		client, err := application.store.GetClientByDevice(context.Background(), deviceID)
		if err == nil && client.Status == "online" {
			return client
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("client %s did not connect", deviceID)
	return model.Client{}
}

func assertAppTunnel(t *testing.T, ctx context.Context, application *App, clientID, payload string) {
	t.Helper()
	echo := startAppEcho(t)
	defer echo.Close()
	port := reserveAppPort(t)
	tunnel, err := application.store.CreateTunnel(ctx, model.Tunnel{
		Name: payload, Protocol: "tcp", ClientID: clientID,
		LocalHost: "127.0.0.1", LocalPort: echo.Addr().(*net.TCPAddr).Port, RemotePort: port,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.tunnels.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}
	assertAppPayload(t, port, payload)
}

func startAppEcho(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener
}

func reserveAppPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func assertAppPayload(t *testing.T, port int, payload string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("relay response=%q want=%q", got, payload)
	}
}

func waitPlainHealth(t *testing.T, baseURL string) {
	t.Helper()
	waitHealthWithClient(t, baseURL, http.DefaultClient)
}

func waitTLSHealth(t *testing.T, baseURL string) {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	waitHealthWithClient(t, baseURL, client)
}

func waitHealthWithClient(t *testing.T, baseURL string, client *http.Client) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/health")
		if err == nil && response.StatusCode == http.StatusOK {
			_ = response.Body.Close()
			return
		}
		if response != nil {
			_ = response.Body.Close()
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s did not start", baseURL)
}

func writeTLSPair(t *testing.T) (string, string, string) {
	t.Helper()
	pair, err := advanced.SelfSignedCertificate()
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	key, ok := pair.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		t.Fatal("unexpected private key type")
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "fullchain.pem")
	keyFile := filepath.Join(dir, "privkey.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile, tlspin.FromCertificate(cert)
}
