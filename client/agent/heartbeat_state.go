package agent

func (a *Agent) setHeartbeatAcks(acks chan string) {
	a.heartbeatMu.Lock()
	a.heartbeatAcks = acks
	a.heartbeatAckEnabled = false
	a.heartbeatMu.Unlock()
}

func (a *Agent) clearHeartbeatAcks(acks chan string) {
	a.heartbeatMu.Lock()
	if a.heartbeatAcks == acks {
		a.heartbeatAcks = nil
		a.heartbeatAckEnabled = false
	}
	a.heartbeatMu.Unlock()
}

func (a *Agent) enableHeartbeatAck() {
	a.heartbeatMu.Lock()
	if a.heartbeatAcks != nil {
		a.heartbeatAckEnabled = true
	}
	a.heartbeatMu.Unlock()
}

func (a *Agent) heartbeatAckRequired() bool {
	a.heartbeatMu.Lock()
	defer a.heartbeatMu.Unlock()
	return a.heartbeatAckEnabled && a.heartbeatAcks != nil
}

func (a *Agent) signalHeartbeatAck(requestID string) {
	a.heartbeatMu.Lock()
	acks := a.heartbeatAcks
	a.heartbeatMu.Unlock()
	if acks == nil {
		return
	}
	select {
	case acks <- requestID:
	default:
	}
}
