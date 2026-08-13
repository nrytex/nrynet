package tunnel

import "context"

func (m *Manager) InvalidateAuthCache() {
	if m.broker != nil {
		m.broker.InvalidateAuthCache()
	}
}

func (m *Manager) InvalidateClientAuthCache(clientID string) {
	if m.broker != nil {
		m.broker.InvalidateClientAuthCache(clientID)
	}
}

func (m *Manager) InvalidateTokenAuthCache(tokenID string) {
	if m.broker != nil {
		m.broker.InvalidateTokenAuthCache(tokenID)
	}
}

func (m *Manager) InvalidateDeviceAuthCache(deviceID string) {
	if m.broker != nil {
		m.broker.InvalidateDeviceAuthCache(deviceID)
	}
}

func (m *Manager) recordTraffic(tunnelID string, upload, download int64) {
	if upload > 0 || download > 0 {
		m.broker.RecordBytes(upload + download)
	}
	if m.traffic != nil {
		m.traffic.add(tunnelID, upload, download)
		return
	}
	_ = m.store.RecordTraffic(context.Background(), tunnelID, upload, download)
}

func (m *Manager) recordEvent(ctx context.Context, level, event, message string, fields map[string]any) {
	if m.events != nil {
		m.events.record(ctx, level, event, message, fields)
		return
	}
	_ = m.store.RecordEvent(ctx, level, event, message, fields)
}
