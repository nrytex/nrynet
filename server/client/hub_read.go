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
			continue
		}
		if message.Type == protocol.TypeUDPPacket {
			h.handleUDPPacket(clientID, message)
			continue
		}
		if message.Type == protocol.TypeVisitorWebRTC {
			go h.handleVisitorWebRTC(clientID, message)
		}
	}
}
