package tunnel

import "time"

const agentReconnectGrace = 15 * time.Second

func (m *Manager) scheduleReconnectCleanup(clientID string) {
	m.mu.Lock()
	if previous := m.reconnectTimers[clientID]; previous != nil {
		previous.Stop()
	}
	m.reconnectEpochs[clientID]++
	epoch := m.reconnectEpochs[clientID]
	m.reconnectTimers[clientID] = time.AfterFunc(agentReconnectGrace, func() {
		m.expireReconnectCleanup(clientID, epoch)
	})
	m.mu.Unlock()
}

func (m *Manager) cancelReconnectCleanup(clientID string) {
	m.mu.Lock()
	if timer := m.reconnectTimers[clientID]; timer != nil {
		timer.Stop()
		delete(m.reconnectTimers, clientID)
	}
	m.reconnectEpochs[clientID]++
	m.mu.Unlock()
}

func (m *Manager) expireReconnectCleanup(clientID string, epoch uint64) {
	m.mu.Lock()
	if m.reconnectEpochs[clientID] != epoch {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	// Do not hold Manager.mu while asking Hub for its status. Hub invokes the
	// disconnect callback while holding its own lock, so reversing that order
	// here would deadlock a reconnect and a cleanup timer.
	if _, connected := m.hub.ConnectedAt(clientID); connected {
		return
	}
	m.mu.Lock()
	if m.reconnectEpochs[clientID] != epoch {
		m.mu.Unlock()
		return
	}
	delete(m.reconnectTimers, clientID)
	m.broker.DisconnectActiveClient(clientID)
	m.mu.Unlock()
}
