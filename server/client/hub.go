package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

const openConnectionRetryWindow = 5 * time.Second

type Hub struct {
	store    *storage.Store
	auth     *auth.Service
	timeout  time.Duration
	upgrader websocket.Upgrader

	mu                       sync.RWMutex
	conns                    map[string]ControlTransport
	connected                map[string]time.Time
	writeQueues              map[string]*controlWriteQueue
	udpHandler               func(string, protocol.ControlMessage)
	visitorWebRTCHandler     func(string, protocol.ControlMessage)
	connectionFailureHandler func(string, protocol.ControlMessage)
	connectHandler           func(string)
	disconnectHandler        func(string)
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
		conns:       make(map[string]ControlTransport),
		connected:   make(map[string]time.Time),
		writeQueues: make(map[string]*controlWriteQueue),
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
	configureWebSocketKeepAlive(conn, h.timeout)
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
	return h.sendOpenConnection(clientID, message)
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
	queue := h.writeQueues[clientID]
	h.mu.RUnlock()
	if conn == nil {
		return errClientOffline
	}
	if queue == nil {
		return errClientOffline
	}
	if err := queue.enqueue(message); err != nil {
		if errors.Is(err, errControlWriteQueueFull) {
			// Queue pressure is not a dead control session. Let the
			// OpenConnection caller retry without dropping the Agent.
			return err
		}
		// A failed control write is a definitive transport failure. Remove the
		// session now instead of waiting for the next heartbeat timeout.
		_ = conn.Close()
		h.unregister(clientID, conn)
		slog.Default().Debug("agent control write failed", "client_id", clientID, "error", fmt.Sprint(err))
		return err
	}
	return nil
}

func (h *Hub) sendOpenConnection(clientID string, message protocol.ControlMessage) error {
	deadline := time.Now().Add(openConnectionRetryWindow)
	var lastErr error
	for {
		lastErr = h.send(clientID, message)
		if lastErr == nil || !isRetryableOpenConnectionError(lastErr) || time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func isRetryableOpenConnectionError(err error) bool {
	return errors.Is(err, errClientOffline) ||
		errors.Is(err, errControlWriteQueueClosed) ||
		errors.Is(err, errControlWriteQueueFull)
}
