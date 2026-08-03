package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/config"
	relayruntime "github.com/nrytex/nrynet/relay/runtime"
)

func TestAppStartsRendezvousAndRelayAPI(t *testing.T) {
	ctx := context.Background()
	cfg := advancedTestConfig(t)
	app, bootstrap, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Run() }()
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	baseURL := "http://" + cfg.Server.Listen
	waitHealth(t, baseURL)
	assertRendezvousObserved(t, cfg.Server.RendezvousListen)
	registerRelayRuntime(t, baseURL, cfg)
	session := login(t, baseURL, bootstrap.Username, cfg.Server.Bootstrap.AdminPassword)
	assertRelayVisible(t, baseURL, session)
}

func TestAppAllowsRemotePlaintextControlListeners(t *testing.T) {
	cfg := advancedTestConfig(t)
	cfg.Server.Listen = "0.0.0.0:0"
	app, _, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = app.Shutdown(context.Background())
}

func advancedTestConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{Server: config.ServerConfig{
		Listen: RendezvousFreeTCP(t), DataListen: RendezvousFreeTCP(t),
		HTTPListen: RendezvousFreeTCP(t), QUICListen: RendezvousFreeUDP(t),
		RendezvousListen: RendezvousFreeUDP(t), Database: t.TempDir() + "/app.db",
		JWTTTL: time.Hour, JWTTTLText: "1h", HeartbeatTimeout: time.Second,
		HeartbeatText: "1s",
		RelayAPIToken: "relay-test-secret",
		Bootstrap:     config.BootstrapConfig{AdminUsername: "admin", AdminPassword: "password-1234"},
	}}
}

func waitHealth(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(baseURL + "/health")
		if err == nil && response.StatusCode == http.StatusOK {
			_ = response.Body.Close()
			return
		}
		if response != nil {
			_ = response.Body.Close()
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("app did not start")
}

func assertRendezvousObserved(t *testing.T, address string) {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	server, _ := net.ResolveUDPAddr("udp", address)
	packet := netx.RendezvousPacket{
		Type: netx.PacketRegister, SessionID: "s", PeerID: "a", WantsPeerID: "b",
	}
	data, _ := json.Marshal(packet)
	_, _ = conn.WriteTo(data, server)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	buffer := make([]byte, 1024)
	n, _, err := conn.ReadFrom(buffer)
	if err != nil {
		t.Fatal(err)
	}
	var observed netx.RendezvousPacket
	if err := json.Unmarshal(buffer[:n], &observed); err != nil {
		t.Fatal(err)
	}
	if observed.Type != netx.PacketObserved || observed.Endpoint.Port == 0 {
		t.Fatalf("unexpected rendezvous response: %#v", observed)
	}
}

func registerRelayRuntime(t *testing.T, baseURL string, cfg config.Config) {
	t.Helper()
	node, err := relayruntime.New(relayruntime.Config{
		ID: "relay-a", Address: "127.0.0.1", ControlAddress: "http://127.0.0.1:7100",
		BrokerAddress: cfg.Server.DataListen, Token: cfg.Server.RelayAPIToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := node.Register(http.DefaultClient, baseURL); err != nil {
		t.Fatal(err)
	}
	if err := node.Heartbeat(http.DefaultClient, baseURL); err != nil {
		t.Fatal(err)
	}
}

func login(t *testing.T, baseURL, username, password string) string {
	t.Helper()
	request := jsonRequest(t, http.MethodPost, baseURL+"/api/auth/login", map[string]string{
		"username": username, "password": password,
	})
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Token == "" {
		t.Fatal("missing session token")
	}
	return payload.Token
}

func assertRelayVisible(t *testing.T, baseURL, session string) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v2/relays", nil)
	request.Header.Set("Authorization", "Bearer "+session)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("relay list status=%d", response.StatusCode)
	}
}

func jsonRequest(t *testing.T, method, url string, value any) *http.Request {
	t.Helper()
	data, _ := json.Marshal(value)
	request, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	return request
}

func RendezvousFreeTCP(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
}

func RendezvousFreeUDP(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	return conn.LocalAddr().String()
}
