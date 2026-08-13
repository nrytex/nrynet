package tunnel

import (
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
	if err := m.hub.OpenConnection(tunnel.ClientID, tunnel, requestID); err != nil {
		m.broker.Cancel(requestID, pending)
		m.recordConnectionFailure(tunnel.ID, requestID, err)
		return
	}
	m.active.Add(1)
	defer m.active.Add(-1)
	err = m.broker.Wait(requestID, pending)
	// Agent setup failures are reported over the control channel and complete
	// this wait immediately. Keep the normal timeout as a final safety net for
	// a disconnected or older Agent that cannot send the report.
	m.recordConnectionFailure(tunnel.ID, requestID, err)
}
