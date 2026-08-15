package tunnel

func (m *Manager) ClientConnected(clientID string) bool {
	_, connected := m.hub.ConnectedAt(clientID)
	return connected
}

func (m *Manager) OnlineClients() int {
	return m.hub.OnlineCount()
}
