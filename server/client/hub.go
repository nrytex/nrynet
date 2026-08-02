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

	mu         sync.RWMutex
	conns      map[string]ControlTransport
	udpHandler func(string, protocol.ControlMessage)
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
		conns: make(map[string]ControlTransport),
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

func (h *Hub) ServeTransport(ctx context.Context, conn ControlTransport, tokenID, ip string) {
	h.serveTransport(ctx, conn, tokenID, ip)
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

func (h *Hub) SendUDPPacket(clientID string, tunnel model.Tunnel, requestID string, payload []byte) error {
	packet := protocol.UDPPacketPayload{
		LocalHost: tunnel.LocalHost,
		LocalPort: tunnel.LocalPort,
		Payload:   payload,
	}
	message, err := protocol.NewMessage(protocol.TypeUDPPacket, requestID, tunnel.ID, packet)
	if err != nil {
		return err
	}
	return h.send(clientID, message)
}

func (h *Hub) SendControl(clientID string, message protocol.ControlMessage) error {
	return h.send(clientID, message)
}

func (h *Hub) SetUDPPacketHandler(handler func(string, protocol.ControlMessage)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.udpHandler = handler
}

func (h *Hub) Disconnect(clientID string) {
	h.mu.Lock()
	conn := h.conns[clientID]
	delete(h.conns, clientID)
	h.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
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
	return conn.WriteJSON(message)
}

func (h *Hub) authenticate(c *gin.Context) (model.Token, error) {
	value := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if value == "" {
		value = c.Query("token")
	}
	return h.auth.AuthenticateAgent(c.Request.Context(), value)
}

func (h *Hub) serve(ctx context.Context, ws *websocket.Conn, tokenID, ip string) {
	h.serveTransport(ctx, &websocketControl{ws: ws}, tokenID, ip)
}

func (h *Hub) serveTransport(ctx context.Context, conn ControlTransport, tokenID, ip string) {
	client, err := h.acceptHello(ctx, conn, tokenID, ip)
	if err != nil {
		_ = conn.WriteJSON(errorMessage(err.Error()))
		_ = conn.Close()
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

func (h *Hub) sendInitialSnapshot(ctx context.Context, conn ControlTransport, clientID string) {
	tunnels, err := h.store.ListClientTunnels(ctx, clientID)
	if err != nil {
		_ = conn.WriteJSON(errorMessage(err.Error()))
		return
	}
	message, err := protocol.NewMessage(
		protocol.TypeTunnelSnapshot,
		"",
		"",
		protocol.TunnelSnapshotPayload{Tunnels: tunnels},
	)
	if err != nil {
		_ = conn.WriteJSON(errorMessage(err.Error()))
		return
	}
	_ = conn.WriteJSON(message)
}

func (h *Hub) acceptHello(ctx context.Context, conn ControlTransport, tokenID, ip string) (model.Client, error) {
	_ = conn.SetReadDeadline(time.Now().Add(h.timeout))
	var message protocol.ControlMessage
	if err := conn.ReadJSON(&message); err != nil {
		return model.Client{}, err
	}
	if message.Type != protocol.TypeHello {
		return model.Client{}, errors.New("first message must be hello")
	}
	payload, err := protocol.DecodePayload[protocol.HelloPayload](message)
	if err != nil {
		return model.Client{}, err
	}
	if payload.DeviceID == "" || payload.Name == "" {
		return model.Client{}, errors.New("device_id and name are required")
	}
	client, err := h.store.UpsertClient(ctx, tokenID, storage.ClientHello{
		Name: payload.Name, DeviceID: payload.DeviceID, IP: ip, OS: payload.OS, Version: payload.Version,
	})
	if err != nil {
		return model.Client{}, err
	}
	if client.Disabled {
		_ = h.store.SetClientStatus(ctx, client.ID, "offline")
		return model.Client{}, errors.New("client is disabled")
	}
	_ = conn.SetReadDeadline(time.Now().Add(h.timeout))
	return client, nil
}

func (h *Hub) register(clientID string, conn ControlTransport) {
	h.mu.Lock()
	old := h.conns[clientID]
	h.conns[clientID] = conn
	h.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

func (h *Hub) unregister(clientID string, conn ControlTransport) {
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

func (h *Hub) readLoop(ctx context.Context, conn ControlTransport, clientID string) {
	for ctx.Err() == nil {
		var message protocol.ControlMessage
		if err := conn.ReadJSON(&message); err != nil {
			return
		}
		if message.Type == protocol.TypeHeartbeat {
			_ = h.store.SetClientStatus(ctx, clientID, "online")
			_ = conn.SetReadDeadline(time.Now().Add(h.timeout))
			continue
		}
		if message.Type == protocol.TypeUDPPacket {
			h.handleUDPPacket(clientID, message)
		}
	}
}

func (h *Hub) handleUDPPacket(clientID string, message protocol.ControlMessage) {
	h.mu.RLock()
	handler := h.udpHandler
	h.mu.RUnlock()
	if handler != nil {
		handler(clientID, message)
	}
}

func errorMessage(text string) protocol.ControlMessage {
	payload, _ := json.Marshal(protocol.ErrorPayload{Message: text})
	return protocol.ControlMessage{Type: protocol.TypeError, Payload: payload}
}

type ControlTransport interface {
	ReadJSON(value any) error
	WriteJSON(value any) error
	Close() error
	SetReadDeadline(time.Time) error
}

type websocketControl struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (c *websocketControl) ReadJSON(value any) error {
	return c.ws.ReadJSON(value)
}

func (c *websocketControl) WriteJSON(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteJSON(value)
}

func (c *websocketControl) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.Close()
}

func (c *websocketControl) SetReadDeadline(deadline time.Time) error {
	return c.ws.SetReadDeadline(deadline)
}
