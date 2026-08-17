package tunnel

import "log/slog"

func (m *Manager) retryPendingVisitors(clientID string) {
	for _, request := range m.broker.PendingRequests(clientID) {
		if err := m.hub.OpenConnection(clientID, request.Tunnel, request.RequestID); err != nil {
			slog.Default().Debug("reissue pending visitor failed", "client_id", clientID,
				"request_id", request.RequestID, "error", err)
		}
	}
}
