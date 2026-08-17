package agent

func (a *Agent) clearSessionEstablished() {
	a.controlMu.Lock()
	a.sessionEstablished = false
	a.controlMu.Unlock()
}

func (a *Agent) markSessionEstablished() {
	a.controlMu.Lock()
	a.sessionEstablished = true
	a.controlMu.Unlock()
}

func (a *Agent) consumeSessionEstablished() bool {
	a.controlMu.Lock()
	defer a.controlMu.Unlock()
	established := a.sessionEstablished
	a.sessionEstablished = false
	return established
}

func (a *Agent) resetSessionReady() {
	a.controlMu.Lock()
	a.sessionReady = false
	a.controlMu.Unlock()
}

func (a *Agent) notifySessionReady() {
	a.controlMu.Lock()
	if a.sessionReady {
		a.controlMu.Unlock()
		return
	}
	a.sessionReady = true
	a.controlMu.Unlock()
	a.notifySessionStarted()
}
