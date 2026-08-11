package agent

const maxActiveVisitorSessions = 32

func (a *Agent) visitorStreamSlots() chan struct{} {
	a.visitorMu.Lock()
	defer a.visitorMu.Unlock()
	if a.visitorSlots == nil {
		a.visitorSlots = make(chan struct{}, visitorMaxTotalStreams)
	}
	return a.visitorSlots
}

func (a *Agent) acquireVisitorSession() bool {
	a.visitorSessionMu.Lock()
	if a.visitorSessionSlots == nil {
		a.visitorSessionSlots = make(chan struct{}, maxActiveVisitorSessions)
	}
	slots := a.visitorSessionSlots
	a.visitorSessionMu.Unlock()
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (a *Agent) releaseVisitorSession() {
	a.visitorSessionMu.Lock()
	slots := a.visitorSessionSlots
	a.visitorSessionMu.Unlock()
	if slots != nil {
		<-slots
	}
}
