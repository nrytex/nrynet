package tunnel

import (
	"log/slog"
	"net"

	"github.com/google/uuid"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
)

func (m *Manager) handleVisitor(tunnel model.Tunnel, visitor net.Conn) {
	if m.tryP2PStream(tunnel, visitor) {
		return
	}
	m.notifyTunnelPath(tunnel, protocol.TunnelPathRelay)
	m.handleRelayedVisitor(tunnel, visitor)
}

func (m *Manager) handleRelayedVisitor(tunnel model.Tunnel, visitor net.Conn) {
	requestID := uuid.NewString()
	pending, err := m.broker.RegisterPending(requestID, visitor, tunnel, func(upload, download int64) {
		m.recordTraffic(tunnel.ID, upload, download)
	})
	if err != nil {
		_ = visitor.Close()
		return
	}
	if !m.broker.TryWorkConnection(requestID) {
		m.openRelayedDataPath(tunnel, requestID)
	}
	m.active.Add(1)
	defer m.active.Add(-1)
	err = m.broker.Wait(requestID, pending)
	// Agent setup failures are reported over the control channel and complete
	// this wait immediately. Keep the normal timeout as a final safety net for
	// a disconnected or older Agent that cannot send the report.
	m.recordConnectionFailure(tunnel.ID, requestID, err)
}

func (m *Manager) openRelayedDataPath(tunnel model.Tunnel, requestID string) {
	if err := m.hub.OpenConnection(tunnel.ClientID, tunnel, requestID); err != nil {
		// Keep the visitor pending. The Agent may be between WebSocket sessions;
		// handleClientConnected will re-issue this command, and Broker.Wait still
		// provides the final timeout when the Agent never returns.
		slog.Default().Debug("visitor command pending during Agent reconnect",
			"client_id", tunnel.ClientID, "tunnel_id", tunnel.ID, "request_id", requestID,
			"error", err)
	}
}
