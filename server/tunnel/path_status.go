package tunnel

import (
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
)

func (m *Manager) notifyTunnelPath(tunnel model.Tunnel, path string) {
	if m.hub == nil || tunnel.ClientID == "" || tunnel.ID == "" {
		return
	}
	if path != protocol.TunnelPathP2P && path != protocol.TunnelPathRelay {
		return
	}
	m.mu.Lock()
	if m.tunnelPaths == nil {
		m.tunnelPaths = make(map[string]string)
	}
	if m.tunnelPaths[tunnel.ID] == path {
		m.mu.Unlock()
		return
	}
	m.tunnelPaths[tunnel.ID] = path
	m.mu.Unlock()
	if err := m.hub.SendTunnelPath(tunnel.ClientID, tunnel.ID, path); err != nil {
		m.mu.Lock()
		if m.tunnelPaths[tunnel.ID] == path {
			delete(m.tunnelPaths, tunnel.ID)
		}
		m.mu.Unlock()
	}
}
