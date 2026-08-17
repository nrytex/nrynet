package relay

import "github.com/nrytex/nrynet/internal/model"

// PendingRequest is a visitor waiting for the Agent to open its local
// service. It is intentionally a snapshot so callers never hold Broker's
// mutex while writing to the Agent control channel.
type PendingRequest struct {
	RequestID string
	Tunnel    model.Tunnel
}

// PendingRequests returns the unpaired visitor requests for one Agent.
func (b *Broker) PendingRequests(clientID string) []PendingRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	requests := make([]PendingRequest, 0)
	for _, entry := range b.pending {
		if entry.tunnel.ClientID != clientID {
			continue
		}
		requests = append(requests, PendingRequest{RequestID: entry.requestID, Tunnel: entry.tunnel})
	}
	return requests
}
