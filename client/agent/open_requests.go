package agent

// beginOpenRequest prevents a reconnect from starting two workers for the
// same visitor while the first worker is still finishing its data handshake.
// The server may replay a pending visitor as soon as a new control session is
// registered, so this guard is needed even though request IDs are unique.
func (a *Agent) beginOpenRequest(requestID string) bool {
	if requestID == "" {
		return true
	}
	a.openMu.Lock()
	defer a.openMu.Unlock()
	if a.openRequests == nil {
		a.openRequests = make(map[string]struct{})
	}
	if _, exists := a.openRequests[requestID]; exists {
		return false
	}
	a.openRequests[requestID] = struct{}{}
	return true
}

func (a *Agent) endOpenRequest(requestID string) {
	if requestID == "" {
		return
	}
	a.openMu.Lock()
	delete(a.openRequests, requestID)
	a.openMu.Unlock()
}
