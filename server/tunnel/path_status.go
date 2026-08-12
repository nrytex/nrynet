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
	// Keep the route even when the Agent is temporarily offline. A reconnect
	// callback replays the cached route, so the desktop does not stay on
	// "unknown" until the next visitor arrives.
	_ = m.hub.SendTunnelPath(tunnel.ClientID, tunnel.ID, path)
}
