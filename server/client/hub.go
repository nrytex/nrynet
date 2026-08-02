package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/protocol"
	"github.com/nat-link/nat-link/internal/storage"
)

var errClientOffline = errors.New("client is not connected")

type Hub struct {
	store    *storage.Store
	auth     *auth.Service
	timeout  time.Duration
	upgrader websocket.Upgrader

	mu    sync.RWMutex
	conns map[string]*controlConn
}

func NewHub(store *storage.Store, authService *auth.Service, timeout time.Duration) *Hub {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Hub{
		store:   store,
		auth:    authService,
		timeout: timeout,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		conns: make(map[string]*controlConn),
	}
}

func (h *Hub) Handler() gin.HandlerFunc {
	return h.Handle
}

func (h *Hub) Handle(c *gin.Context) {
	token, err := h.authenticate(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.serve(c.Request.Context(), conn, token.ID, c.ClientIP())
}

func (h *Hub) SendSnapshot(clientID string, tunnels []model.Tunnel) error {
	message, err := protocol.NewMessage(protocol.TypeTunnelSnapshot, "", "", protocol.TunnelSnapshotPayload{Tunnels: tunnels})
	if err != nil {
		return err
	}
	return h.send(clientID, message)
}

func (h *Hub) OpenConnection(clientID string, tunnel model.Tunnel, requestID string) error {
	payload := protocol.OpenConnectionPayload{LocalHost: tunnel.LocalHost, LocalPort: tunnel.LocalPort}
	message, err := protocol.NewMessage(protocol.TypeOpenConnection, requestID, tunnel.ID, payload)
	if err != nil {
		return err
	}
	return h.send(clientID, message)
}

func (h *Hub) Disconnect(clientID string) {
	h.mu.Lock()
	conn := h.conns[clientID]
	delete(h.conns, clientID)
	h.mu.Unlock()
	if conn != nil {
		_ = conn.close()
	}
}

func (h *Hub) OnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

func (h *Hub) send(clientID string, message protocol.ControlMessage) error {
	h.mu.RLock()
	conn := h.conns[clientID]
	h.mu.RUnlock()
	if conn == nil {
		return errClientOffline
	}
	return conn.writeJSON(message)
}

func (h *Hub) authenticate(c *gin.Context) (model.Token, error) {
	value := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if value == "" {
		value = c.Query("token")
	}
	return h.auth.AuthenticateAgent(c.Request.Context(), value)
}

func (h *Hub) serve(ctx context.Context, ws *websocket.Conn, tokenID, ip string) {
	conn, client, err := h.acceptHello(ctx, ws, tokenID, ip)
	if err != nil {
		_ = ws.WriteJSON(errorMessage(err.Error()))
		_ = ws.Close()
		return
	}
	h.register(client.ID, conn)
	_ = h.store.RecordEvent(ctx, "info", "client.connected", "Client connected", map[string]any{
		"client_id": client.ID, "device_id": client.DeviceID, "ip": client.IP,
	})
	defer h.unregister(client.ID, conn)
	h.sendInitialSnapshot(ctx, conn, client.ID)
	h.readLoop(ctx, conn, client.ID)
}

func (h *Hub) sendInitialSnapshot(ctx context.Context, conn *controlConn, clientID string) {
	tunnels, err := h.store.ListClientTunnels(ctx, clientID)
	if err != nil {
		_ = conn.writeJSON(errorMessage(err.Error()))
		return
	}
	message, err := protocol.NewMessage(
		protocol.TypeTunnelSnapshot,
		"",
		"",
		protocol.TunnelSnapshotPayload{Tunnels: tunnels},
	)
	if err != nil {
		_ = conn.writeJSON(errorMessage(err.Error()))
		return
	}
	_ = conn.writeJSON(message)
}

func (h *Hub) acceptHello(ctx context.Context, ws *websocket.Conn, tokenID, ip string) (*controlConn, model.Client, error) {
	_ = ws.SetReadDeadline(time.Now().Add(h.timeout))
	var message protocol.ControlMessage
	if err := ws.ReadJSON(&message); err != nil {
		return nil, model.Client{}, err
	}
	if message.Type != protocol.TypeHello {
		return nil, model.Client{}, errors.New("first message must be hello")
	}
	payload, err := protocol.DecodePayload[protocol.HelloPayload](message)
	if err != nil {
		return nil, model.Client{}, err
	}
	if payload.DeviceID == "" || payload.Name == "" {
		return nil, model.Client{}, errors.New("device_id and name are required")
	}
	client, err := h.store.UpsertClient(ctx, tokenID, storage.ClientHello{
		Name: payload.Name, DeviceID: payload.DeviceID, IP: ip, OS: payload.OS, Version: payload.Version,
	})
	if err != nil {
		return nil, model.Client{}, err
	}
	if client.Disabled {
		_ = h.store.SetClientStatus(ctx, client.ID, "offline")
		return nil, model.Client{}, errors.New("client is disabled")
	}
	_ = ws.SetReadDeadline(time.Now().Add(h.timeout))
	return &controlConn{ws: ws}, client, nil
}

func (h *Hub) register(clientID string, conn *controlConn) {
	h.mu.Lock()
	old := h.conns[clientID]
	h.conns[clientID] = conn
	h.mu.Unlock()
	if old != nil {
		_ = old.close()
	}
}

func (h *Hub) unregister(clientID string, conn *controlConn) {
	h.mu.Lock()
	isCurrent := h.conns[clientID] == conn
	if isCurrent {
		delete(h.conns, clientID)
	}
	h.mu.Unlock()
	if !isCurrent {
		return
	}
	_ = h.store.SetClientStatus(context.Background(), clientID, "offline")
	_ = h.store.RecordEvent(context.Background(), "info", "client.disconnected",
		"Client disconnected", map[string]any{"client_id": clientID})
}

func (h *Hub) readLoop(ctx context.Context, conn *controlConn, clientID string) {
	for ctx.Err() == nil {
		var message protocol.ControlMessage
		if err := conn.ws.ReadJSON(&message); err != nil {
			return
		}
		if message.Type == protocol.TypeHeartbeat {
			_ = h.store.SetClientStatus(ctx, clientID, "online")
			_ = conn.ws.SetReadDeadline(time.Now().Add(h.timeout))
		}
	}
}

func errorMessage(text string) protocol.ControlMessage {
	payload, _ := json.Marshal(protocol.ErrorPayload{Message: text})
	return protocol.ControlMessage{Type: protocol.TypeError, Payload: payload}
}

type controlConn struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (c *controlConn) writeJSON(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteJSON(value)
}

func (c *controlConn) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.Close()
}
