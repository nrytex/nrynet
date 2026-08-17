package client

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

func (h *Hub) readLoop(ctx context.Context, conn ControlTransport, clientID string) {
	for ctx.Err() == nil {
		var message protocol.ControlMessage
		if err := conn.ReadJSON(&message); err != nil {
			slog.Default().Debug("agent control read ended", "client_id", clientID, "error", fmt.Sprint(err))
			return
		}
		if message.Type == protocol.TypeHeartbeat {
			// Refresh the deadline before any database work. Traffic accounting
			// uses the same single SQLite connection and must not make a healthy
			// control channel time out while waiting on that connection.
			_ = conn.SetReadDeadline(time.Now().Add(h.timeout))
			if err := h.sendHeartbeatAck(clientID, conn, message.RequestID); err != nil {
				slog.Default().Debug("agent heartbeat acknowledgement failed", "client_id", clientID, "error", fmt.Sprint(err))
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(h.timeout))
			continue
		}
		if message.Type == protocol.TypeUDPPacket {
			h.dispatchUDP(clientID, message)
			continue
		}
		if message.Type == protocol.TypeVisitorWebRTC {
			go h.handleVisitorWebRTC(clientID, message)
			continue
		}
		if message.Type == protocol.TypeConnectionFailed {
			h.handleConnectionFailure(clientID, message)
		}
	}
}

func (h *Hub) sendHeartbeatAck(clientID string, conn ControlTransport, requestID string) error {
	h.mu.RLock()
	current := h.conns[clientID]
	queue := h.writeQueues[clientID]
	h.mu.RUnlock()
	if current != conn || queue == nil {
		return errClientOffline
	}
	message := protocol.ControlMessage{Type: protocol.TypeHeartbeatAck, RequestID: requestID}
	ctx, cancel := context.WithTimeout(context.Background(), heartbeatWriteTimeout)
	err := queue.enqueueWait(ctx, message)
	cancel()
	if err == nil {
		return nil
	}
	_ = conn.Close()
	h.unregister(clientID, conn)
	return err
}

func (h *Hub) dispatchUDP(clientID string, message protocol.ControlMessage) {
	h.mu.RLock()
	handler := h.udpHandler
	h.mu.RUnlock()
	if handler != nil {
		handler(clientID, message)
	}
}
