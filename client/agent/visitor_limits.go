package agent

func (a *Agent) visitorStreamSlots() chan struct{} {
	a.visitorMu.Lock()
	defer a.visitorMu.Unlock()
	if a.visitorSlots == nil {
		a.visitorSlots = make(chan struct{}, visitorMaxTotalStreams)
	}
	return a.visitorSlots
}
