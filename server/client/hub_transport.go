package client

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
	"github.com/nrytex/nrynet/internal/storage"
)

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
	queue := newControlWriteQueue(conn, func() {
		_ = conn.Close()
		go h.unregister(client.ID, conn)
	})
	h.register(client.ID, conn, queue)
	defer func() {
		queue.close()
		h.mu.Lock()
		if h.writeQueues[client.ID] == queue {
			delete(h.writeQueues, client.ID)
		}
		h.mu.Unlock()
	}()
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
	h.sendInitialSnapshot(ctx, client.ID)
	h.mu.RLock()
	connectHandler := h.connectHandler
	h.mu.RUnlock()
	if connectHandler != nil {
		connectHandler(client.ID)
	}
	// Listing tunnels may wait briefly on SQLite while traffic records are
	// flushed. Start the heartbeat window after the initial snapshot, not from
	// the hello deadline.
	_ = conn.SetReadDeadline(time.Now().Add(h.timeout))
	h.readLoop(ctx, conn, client.ID)
}

func (h *Hub) sendInitialSnapshot(ctx context.Context, clientID string) {
	tunnels, err := h.store.ListClientTunnels(ctx, clientID)
	if err != nil {
		_ = h.send(clientID, errorMessage(err.Error()))
		return
	}
	message, err := protocol.NewMessage(
		protocol.TypeTunnelSnapshot,
		"",
		"",
		protocol.TunnelSnapshotPayload{Tunnels: tunnels},
	)
	if err != nil {
		_ = h.send(clientID, errorMessage(err.Error()))
		return
	}
	_ = h.send(clientID, message)
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

func (h *Hub) register(clientID string, conn ControlTransport, queue *controlWriteQueue) {
	h.mu.Lock()
	old := h.conns[clientID]
	oldQueue := h.writeQueues[clientID]
	h.conns[clientID] = conn
	h.connected[clientID] = time.Now().UTC()
	h.writeQueues[clientID] = queue
	handler := h.disconnectHandler
	h.mu.Unlock()
	if oldQueue != nil {
		oldQueue.close()
	}
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
	queue := h.writeQueues[clientID]
	delete(h.writeQueues, clientID)
	if handler != nil {
		// The configured handler only closes broker/UDP state and must not call
		// back into Hub while this lock is held.
		handler(clientID)
	}
	h.mu.Unlock()
	if queue != nil {
		queue.close()
	}
	_ = h.store.RecordEvent(context.Background(), "info", "client.disconnected",
		"Client disconnected", map[string]any{"client_id": clientID})
}
