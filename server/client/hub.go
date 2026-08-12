package client

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/nrytex/nrynet/internal/auth"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
	"github.com/nrytex/nrynet/internal/storage"
)

var errClientOffline = errors.New("client is not connected")

type Hub struct {
	store    *storage.Store
	auth     *auth.Service
	timeout  time.Duration
	upgrader websocket.Upgrader

	mu                   sync.RWMutex
	conns                map[string]ControlTransport
	connected            map[string]time.Time
	udpHandler           func(string, protocol.ControlMessage)
	visitorWebRTCHandler func(string, protocol.ControlMessage)
	disconnectHandler    func(string)
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
		conns:     make(map[string]ControlTransport),
		connected: make(map[string]time.Time),
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

func (h *Hub) SendTunnelPath(clientID, tunnelID, path string) error {
	message, err := protocol.NewMessage(
		protocol.TypeTunnelPath,
		"",
		tunnelID,
		protocol.TunnelPathPayload{Path: path},
	)
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

func (h *Hub) Disconnect(clientID string) bool {
	h.mu.RLock()
	conn := h.conns[clientID]
	h.mu.RUnlock()
	if conn != nil {
		_ = conn.Close()
		h.unregister(clientID, conn)
		return true
	}
	return false
}

func (h *Hub) ConnectedAt(clientID string) (time.Time, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	connectedAt, ok := h.connected[clientID]
	return connectedAt, ok
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
	if err := conn.WriteJSON(message); err != nil {
		// A failed control write is a definitive transport failure. Remove the
		// session now instead of waiting for the next heartbeat timeout.
		_ = conn.Close()
		h.unregister(clientID, conn)
		return err
	}
	return nil
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
	_ = h.store.SetClientStatus(ctx, client.ID, "online")
	_ = h.store.RecordEvent(ctx, "info", "client.connected", "Client connected", map[string]any{
		"client_id": client.ID, "device_id": client.DeviceID, "ip": client.IP,
	})
	// A WebSocket/QUIC handler owns the transport after the handshake. When
	// the read loop exits (for example after a heartbeat timeout), close the
	// underlying connection as well as removing it from the hub. Without this
	// close the client can continue writing heartbeats into a socket that the
	// server no longer services, leaving the desktop UI stuck on "connected".
	defer func() {
		_ = conn.Close()
		h.unregister(client.ID, conn)
	}()
	h.sendInitialSnapshot(ctx, conn, client.ID)
	// Listing tunnels may wait briefly on SQLite while traffic records are
	// flushed. Start the heartbeat window after the initial snapshot, not from
	// the hello deadline.
	_ = conn.SetReadDeadline(time.Now().Add(h.timeout))
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
	h.connected[clientID] = time.Now().UTC()
	handler := h.disconnectHandler
	h.mu.Unlock()
	if old != nil {
		_ = old.Close()
		if handler != nil {
			// A replacement session invalidates active broker streams even though
			// the old transport's deferred cleanup is intentionally ignored.
			handler(clientID)
		}
	}
}

func (h *Hub) unregister(clientID string, conn ControlTransport) {
	h.mu.Lock()
	isCurrent := h.conns[clientID] == conn
	if !isCurrent {
		h.mu.Unlock()
		return
	}
	// Keep the hub lock through the offline transition. A reconnect cannot
	// register between this session's removal and the database write, which
	// would otherwise let the old session overwrite the new session's online
	// status.
	handler := h.disconnectHandler
	_ = h.store.SetClientStatus(context.Background(), clientID, "offline")
	delete(h.conns, clientID)
	delete(h.connected, clientID)
	if handler != nil {
		// The configured handler only closes broker/UDP state and must not call
		// back into Hub while this lock is held.
		handler(clientID)
	}
	h.mu.Unlock()
	_ = h.store.RecordEvent(context.Background(), "info", "client.disconnected",
		"Client disconnected", map[string]any{"client_id": clientID})
}
