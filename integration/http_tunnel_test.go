package integration

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	clientagent "github.com/nat-link/nat-link/client/agent"
	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/internal/model"
	clienthub "github.com/nat-link/nat-link/server/client"
	"github.com/nat-link/nat-link/server/gateway"
	"github.com/nat-link/nat-link/server/relay"
	serverTunnel "github.com/nat-link/nat-link/server/tunnel"
)

func TestHTTPAndHTTPSTunnelsEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 2*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	defer control.Close()
	dataListener := listenTCP(t)
	broker := relay.NewBroker(authService, store, 3*time.Second)
	runBroker(t, dataListener, broker)
	agent := newHTTPAgent(t, control.URL, dataListener.Addr().String(), cleartext)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "http-device")

	plain := httptest.NewServer(responseHandler("plain-response"))
	defer plain.Close()
	secure := httptest.NewTLSServer(responseHandler("secure-response"))
	defer secure.Close()
	websocketServer := httptest.NewServer(websocketHandler())
	defer websocketServer.Close()
	plainTunnel := createDomainTunnel(t, store, client.ID, "http", "plain.test", serverPort(t, plain.URL))
	secureTunnel := createDomainTunnel(t, store, client.ID, "https", "secure.test", serverPort(t, secure.URL))
	websocketTunnel := createDomainTunnel(t, store, client.ID, "http", "ws.test", serverPort(t, websocketServer.URL))
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	if err := manager.StartTunnel(ctx, plainTunnel.ID); err != nil {
		t.Fatal(err)
	}
	if err := manager.StartTunnel(ctx, secureTunnel.ID); err != nil {
		t.Fatal(err)
	}
	if err := manager.StartTunnel(ctx, websocketTunnel.ID); err != nil {
		t.Fatal(err)
	}
	listener := listenTCP(t)
	defer listener.Close()
	webGateway := gateway.New(store, manager)
	go func() { _ = webGateway.Run(listener) }()

	assertPlainHTTP(t, listener.Addr().String())
	assertHTTPS(t, listener.Addr().String())
	assertWebSocket(t, listener.Addr().String())
}

func responseHandler(body string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-NAT-Link-Host", request.Host)
		_, _ = io.WriteString(writer, body)
	})
}

func websocketHandler() http.Handler {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		messageType, data, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(messageType, data)
		}
	})
}

func newHTTPAgent(t *testing.T, serverURL, dataAddress, token string) *clientagent.Agent {
	t.Helper()
	clientConfig := config.ClientConfig{
		ServerURL:   strings.Replace(serverURL, "http://", "ws://", 1) + "/agent/connect",
		DataAddress: dataAddress, Token: token, Name: "http-agent", DeviceID: "http-device",
	}
	client, err := clientagent.New(clientagent.NewOptions(config.Config{Client: clientConfig}, "test"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func createDomainTunnel(t *testing.T, store interface {
	CreateTunnel(context.Context, model.Tunnel) (model.Tunnel, error)
}, clientID, protocol, domain string, localPort int) model.Tunnel {
	t.Helper()
	tunnel, err := store.CreateTunnel(context.Background(), model.Tunnel{
		Name: domain, Protocol: protocol, ClientID: clientID,
		LocalHost: "127.0.0.1", LocalPort: localPort, Domain: domain,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tunnel
}

func serverPort(t *testing.T, rawURL string) int {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	_, value, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(value)
	return port
}

func assertPlainHTTP(t *testing.T, address string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: plain.test\r\nConnection: close\r\n\r\n")
	response, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response), "plain-response") {
		t.Fatalf("plain response missing body: %s", response)
	}
}

func assertHTTPS(t *testing.T, address string) {
	t.Helper()
	conn, err := tls.Dial("tcp", address, &tls.Config{
		InsecureSkipVerify: true, ServerName: "secure.test", MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: secure.test\r\nConnection: close\r\n\r\n")
	response, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response), "secure-response") {
		t.Fatalf("secure response missing body: %s", response)
	}
}

func assertWebSocket(t *testing.T, address string) {
	t.Helper()
	dialer := websocket.Dialer{NetDialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var netDialer net.Dialer
		return netDialer.DialContext(ctx, network, address)
	}}
	conn, _, err := dialer.Dial("ws://ws.test/socket", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("websocket-relay")); err != nil {
		t.Fatal(err)
	}
	_, response, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != "websocket-relay" {
		t.Fatalf("websocket response=%q", response)
	}
}
