package client

import (
	"context"
	"time"
)

const clientEventWriteTimeout = 2 * time.Second

func (h *Hub) markClientOnline(clientID string, conn ControlTransport) {
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	if !h.isCurrentConnection(clientID, conn) {
		return
	}
	_ = h.store.SetClientStatus(context.Background(), clientID, "online")
}

func (h *Hub) markClientOffline(clientID string) {
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	if h.hasConnection(clientID) {
		return
	}
	_ = h.store.SetClientStatus(context.Background(), clientID, "offline")
}

func (h *Hub) isCurrentConnection(clientID string, conn ControlTransport) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.conns[clientID] == conn
}

func (h *Hub) hasConnection(clientID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.conns[clientID] != nil
}

func (h *Hub) recordClientEvent(event, message string, fields map[string]any) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), clientEventWriteTimeout)
		defer cancel()
		_ = h.store.RecordEvent(ctx, "info", event, message, fields)
	}()
}
