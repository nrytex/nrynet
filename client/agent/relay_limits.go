package agent

import "context"

const maxActiveRelayConnections = 128

func (a *Agent) acquireRelaySlot(ctx context.Context) bool {
	a.relayMu.Lock()
	if a.relaySlots == nil {
		a.relaySlots = make(chan struct{}, maxActiveRelayConnections)
	}
	slots := a.relaySlots
	a.relayMu.Unlock()
	select {
	case slots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (a *Agent) releaseRelaySlot() {
	a.relayMu.Lock()
	slots := a.relaySlots
	a.relayMu.Unlock()
	if slots != nil {
		<-slots
	}
}
