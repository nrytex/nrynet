package tunnel

import "github.com/nat-link/nat-link/internal/protocol"

func (m *Manager) HandleUDPPacket(clientID string, message protocol.ControlMessage) {
	payload, err := protocol.DecodePayload[protocol.UDPPacketPayload](message)
	if err != nil || len(payload.Payload) == 0 {
		return
	}
	runtime := m.udpRuntimeFor(clientID, message.TunnelID)
	if runtime == nil {
		return
	}
	_ = runtime.sendToVisitor(message.RequestID, payload.Payload)
}

func (m *Manager) udpRuntimeFor(clientID, tunnelID string) *udpRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime := m.udpRuntimes[tunnelID]
	if runtime == nil || (clientID != "" && runtime.tunnel.ClientID != clientID) {
		return nil
	}
	return runtime
}

func (m *Manager) removeUDPSession(tunnelID, sessionID string) {
	runtime := m.udpRuntimeFor("", tunnelID)
	if runtime != nil {
		runtime.removeSession(sessionID)
	}
}

func (r *udpRuntime) removeSession(sessionID string) {
	r.mu.Lock()
	session := r.byID[sessionID]
	if session == nil {
		r.mu.Unlock()
		return
	}
	delete(r.sessions, session.addr.String())
	delete(r.byID, sessionID)
	r.mu.Unlock()
	session.closeP2P()
	r.manager.active.Add(-1)
}
