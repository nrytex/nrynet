package relay

import (
	"context"
	"crypto/subtle"
	"net"
	"time"
)

func (b *Broker) handleRelayVisitor(visitor net.Conn, handshake initialHandshake) {
	b.mu.Lock()
	token, handler := b.relayToken, b.relayVisitor
	b.mu.Unlock()
	if handler == nil || handshake.NodeID == "" || handshake.TunnelID == "" || handshake.VisitorAddr == "" ||
		token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(handshake.Token)) != 1 {
		_ = visitor.Close()
		return
	}
	if err := handler(handshake.NodeID, handshake.TunnelID, handshake.VisitorAddr, visitor); err != nil {
		b.recordRejected("relay visitor rejected", err)
		_ = visitor.Close()
	}
}

func (b *Broker) recordRejected(message string, err error) {
	if err == nil || !b.allowRejectionEvent() {
		return
	}
	_ = b.store.RecordEvent(context.Background(), "warn", "relay.rejected", message, map[string]any{
		"error": err.Error(),
	})
}

func (b *Broker) allowRejectionEvent() bool {
	now := time.Now()
	b.rejectMu.Lock()
	defer b.rejectMu.Unlock()
	if b.rejectWindow.IsZero() || now.Sub(b.rejectWindow) >= time.Second {
		b.rejectWindow = now
		b.rejectCount = 0
	}
	if b.rejectCount >= 20 {
		return false
	}
	b.rejectCount++
	return true
}
