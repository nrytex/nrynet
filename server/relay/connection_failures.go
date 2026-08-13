package relay

import "errors"

func (b *Broker) Fail(clientID, requestID string, cause error) {
	if requestID == "" {
		return
	}
	if cause == nil {
		cause = errors.New("agent failed to open the connection")
	}
	b.mu.Lock()
	pending := b.pending[requestID]
	if pending != nil && pending.tunnel.ClientID == clientID {
		delete(b.pending, requestID)
	} else {
		pending = nil
	}
	b.mu.Unlock()
	if pending != nil {
		_ = pending.visitor.Close()
		select {
		case pending.done <- cause:
		default:
		}
	}
	b.connections.closeRequest(clientID, requestID)
}
