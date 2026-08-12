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
