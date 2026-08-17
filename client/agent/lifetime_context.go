package agent

import "context"

func (a *Agent) beginRun(ctx context.Context) {
	a.runMu.Lock()
	a.runCtx = ctx
	a.runMu.Unlock()
}

func (a *Agent) endRun() {
	a.runMu.Lock()
	a.runCtx = nil
	a.runMu.Unlock()
}

func (a *Agent) relayContext(fallback context.Context) context.Context {
	a.runMu.RLock()
	ctx := a.runCtx
	a.runMu.RUnlock()
	if ctx == nil {
		return fallback
	}
	return ctx
}
