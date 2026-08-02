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

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/protocol"
	"github.com/nat-link/nat-link/internal/storage"
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
	if client.TokenID != token.ID || hub.OnlineCount() != 1 {
		t.Fatalf("agent not registered: %#v online=%d", client, hub.OnlineCount())
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
	store, err := storage.Open(t.TempDir() + "/test.db")
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
