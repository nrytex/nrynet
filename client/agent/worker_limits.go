package agent

import "context"

const maxActiveStreamWorkers = 128

func (a *Agent) acquireStreamWorker(ctx context.Context) bool {
	a.streamWorkerMu.Lock()
	if a.streamWorkerSlots == nil {
		a.streamWorkerSlots = make(chan struct{}, maxActiveStreamWorkers)
	}
	slots := a.streamWorkerSlots
	a.streamWorkerMu.Unlock()
	select {
	case slots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (a *Agent) releaseStreamWorker() {
	a.streamWorkerMu.Lock()
	slots := a.streamWorkerSlots
	a.streamWorkerMu.Unlock()
	if slots != nil {
		<-slots
	}
}
