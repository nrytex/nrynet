package tunnel

import (
	"context"
	"fmt"
)

func (m *Manager) recordConnectionFailure(tunnelID, requestID string, err error) {
	if err == nil {
		return
	}
	_ = m.store.RecordEvent(
		context.Background(),
		"warn",
		"connection.failed",
		fmt.Sprintf("Tunnel connection failed: %v", err),
		map[string]any{"tunnel_id": tunnelID, "request_id": requestID, "error": err.Error()},
	)
}
