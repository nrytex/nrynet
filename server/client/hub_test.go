package client

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/nrytex/nrynet/internal/auth"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
	"github.com/nrytex/nrynet/internal/storage"
)

func TestHubAcceptsAgentAndSendsCommands(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, authService := newHubStore(t)
	token, cleartext, err := authService.CreateAgentToken(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	hub := NewHub(store, authService, time.Second)
	server := httptest.NewServer(routerWithHub(hub))
	defer server.Close()

	ws := dialHub(t, server.URL, cleartext)
	defer ws.Close()
	writeHello(t, ws, "device-1")
	expectMessageType(t, ws, protocol.TypeTunnelSnapshot)
	client, err := store.GetClientByDevice(context.Background(), "device-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hub.Disconnect(client.ID) })
	if client.TokenID != token.ID || hub.OnlineCount() != 1 {
		t.Fatalf("agent not registered: %#v online=%d", client, hub.OnlineCount())
	}
	connectedAt, connected := hub.ConnectedAt(client.ID)
	if !connected || time.Since(connectedAt) > time.Second {
		t.Fatalf("client connection time was not tracked: %v %v", connectedAt, connected)
	}

	tunnel := model.Tunnel{ID: "tun-1", LocalHost: "127.0.0.1", LocalPort: 8080}
	if err := hub.OpenConnection(client.ID, tunnel, "req-1"); err != nil {
		t.Fatal(err)
	}
	var got protocol.ControlMessage
	if err := ws.ReadJSON(&got); err != nil {
		t.Fatal(err)
	}
	payload, err := protocol.DecodePayload[protocol.OpenConnectionPayload](got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != protocol.TypeOpenConnection || payload.LocalPort != 8080 {
		t.Fatalf("unexpected command: %#v %#v", got, payload)
	}
}

func TestHubRejectsDisabledClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, authService := newHubStore(t)
	token, cleartext, err := authService.CreateAgentToken(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	existing, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "old", DeviceID: "device-2", IP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	disabled := true
	if err := store.UpdateClient(context.Background(), existing.ID, "", &disabled); err != nil {
		t.Fatal(err)
	}
	hub := NewHub(store, authService, time.Second)
	server := httptest.NewServer(routerWithHub(hub))
	defer server.Close()

	ws := dialHub(t, server.URL, cleartext)
	defer ws.Close()
	writeHello(t, ws, "device-2")
	var got protocol.ControlMessage
	if err := ws.ReadJSON(&got); err != nil {
		t.Fatal(err)
	}
	if got.Type != protocol.TypeError || hub.OnlineCount() != 0 {
		t.Fatalf("disabled client was accepted: %#v", got)
	}
}

func TestHubRejectsDeviceTakeoverByDifferentToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, authService := newHubStore(t)
	first, firstValue, err := authService.CreateAgentToken(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	_, secondValue, err := authService.CreateAgentToken(context.Background(), "second")
	if err != nil {
		t.Fatal(err)
	}
	hub := NewHub(store, authService, time.Second)
	server := httptest.NewServer(routerWithHub(hub))
	defer server.Close()

	original := dialHub(t, server.URL, firstValue)
	defer original.Close()
	writeHello(t, original, "owned-device")
	expectMessageType(t, original, protocol.TypeTunnelSnapshot)
	attacker := dialHub(t, server.URL, secondValue)
	defer attacker.Close()
	writeHello(t, attacker, "owned-device")
	expectMessageType(t, attacker, protocol.TypeError)
	client, err := store.GetClientByDevice(context.Background(), "owned-device")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hub.Disconnect(client.ID) })
	if client.TokenID != first.ID || hub.OnlineCount() != 1 {
		t.Fatalf("device ownership changed: client=%+v online=%d", client, hub.OnlineCount())
	}
}

func TestHubDisconnectMarksClientOffline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, authService := newHubStore(t)
	_, cleartext, err := authService.CreateAgentToken(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	hub := NewHub(store, authService, time.Second)
	server := httptest.NewServer(routerWithHub(hub))
	defer server.Close()

	ws := dialHub(t, server.URL, cleartext)
	defer ws.Close()
	writeHello(t, ws, "disconnect-device")
	expectMessageType(t, ws, protocol.TypeTunnelSnapshot)
	client, err := store.GetClientByDevice(context.Background(), "disconnect-device")
	if err != nil {
		t.Fatal(err)
	}

	hub.Disconnect(client.ID)
	disconnected, err := store.GetClient(context.Background(), client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if disconnected.Status != "offline" || hub.OnlineCount() != 0 {
		t.Fatalf("disconnect left stale state: client=%+v online=%d", disconnected, hub.OnlineCount())
	}
}

func TestHubTimeoutClosesClientTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, authService := newHubStore(t)
	_, cleartext, err := authService.CreateAgentToken(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	hub := NewHub(store, authService, 50*time.Millisecond)
	server := httptest.NewServer(routerWithHub(hub))
	defer server.Close()

	ws := dialHub(t, server.URL, cleartext)
	defer ws.Close()
	writeHello(t, ws, "timeout-device")
	expectMessageType(t, ws, protocol.TypeTunnelSnapshot)
	client, err := store.GetClientByDevice(context.Background(), "timeout-device")
	if err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(time.Second))
	var message protocol.ControlMessage
	if err := ws.ReadJSON(&message); err == nil {
		t.Fatal("server heartbeat timeout left the control transport open")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, getErr := store.GetClient(context.Background(), client.ID)
		if getErr == nil && current.Status == "offline" && hub.OnlineCount() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, _ := store.GetClient(context.Background(), client.ID)
	t.Fatalf("timeout left stale online state: client=%+v online=%d", current, hub.OnlineCount())
}

func TestHubRefreshesWebSocketDeadlineAfterAgentPing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, authService := newHubStore(t)
	_, cleartext, err := authService.CreateAgentToken(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	hub := NewHub(store, authService, 200*time.Millisecond)
	server := httptest.NewServer(routerWithHub(hub))
	defer server.Close()
	ws := dialHub(t, server.URL, cleartext)
	defer ws.Close()
	writeHello(t, ws, "ping-device")
	expectMessageType(t, ws, protocol.TypeTunnelSnapshot)
	if err := ws.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := ws.WriteJSON(protocol.ControlMessage{Type: protocol.TypeHeartbeat}); err != nil {
		t.Fatalf("ping did not keep WebSocket alive: %v", err)
	}
}

func TestHubTokenRotationRejectsOldTokenAndAcceptsNewToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, authService := newHubStore(t)
	_, oldValue, err := authService.CreateAgentToken(context.Background(), "old")
	if err != nil {
		t.Fatal(err)
	}
	hub := NewHub(store, authService, time.Second)
	server := httptest.NewServer(routerWithHub(hub))
	defer server.Close()

	original := dialHub(t, server.URL, oldValue)
	writeHello(t, original, "rotated-device")
	expectMessageType(t, original, protocol.TypeTunnelSnapshot)
	client, err := store.GetClientByDevice(context.Background(), "rotated-device")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hub.Disconnect(client.ID) })
	newToken, newValue, err := authService.CreateAgentToken(context.Background(), "new")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateClientToken(context.Background(), client.ID, newToken.ID); err != nil {
		t.Fatal(err)
	}
	hub.Disconnect(client.ID)
	_ = original.Close()

	stale := dialHub(t, server.URL, oldValue)
	defer stale.Close()
	writeHello(t, stale, "rotated-device")
	expectMessageType(t, stale, protocol.TypeError)

	replacement := dialHub(t, server.URL, newValue)
	defer replacement.Close()
	writeHello(t, replacement, "rotated-device")
	expectMessageType(t, replacement, protocol.TypeTunnelSnapshot)
	connected, err := store.GetClient(context.Background(), client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if connected.Status != "online" || hub.OnlineCount() != 1 {
		t.Fatalf("new token did not restore the client: client=%+v online=%d", connected, hub.OnlineCount())
	}
}

func expectMessageType(t *testing.T, ws *websocket.Conn, want string) {
	t.Helper()
	var got protocol.ControlMessage
	if err := ws.ReadJSON(&got); err != nil {
		t.Fatal(err)
	}
	if got.Type != want {
		t.Fatalf("want message %q, got %#v", want, got)
	}
}

func newHubStore(t *testing.T) (*storage.Store, *auth.Service) {
	t.Helper()
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	authService, err := auth.New(context.Background(), store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return store, authService
}

func routerWithHub(hub *Hub) *gin.Engine {
	router := gin.New()
	router.GET("/agent/connect", hub.Handle)
	return router
}

func dialHub(t *testing.T, url, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(url, "http") + "/agent/connect"
	header := map[string][]string{"Authorization": {"Bearer " + token}}
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func writeHello(t *testing.T, ws *websocket.Conn, deviceID string) {
	t.Helper()
	payload, err := json.Marshal(protocol.HelloPayload{Name: "agent", DeviceID: deviceID})
	if err != nil {
		t.Fatal(err)
	}
	message := protocol.ControlMessage{Type: protocol.TypeHello, Payload: payload}
	if err := ws.WriteJSON(message); err != nil {
		t.Fatal(err)
	}
}
