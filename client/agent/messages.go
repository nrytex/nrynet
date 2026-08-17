package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

func (a *Agent) heartbeat(ctx context.Context, conn controlConn) error {
	interval := a.options.HeartbeatInterval
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}
	requestID, err := a.sendHeartbeatRequest(conn)
	if err != nil {
		return err
	}
	if err := a.waitHeartbeatAckIfRequired(ctx, interval, requestID); err != nil {
		return fmt.Errorf("wait for heartbeat acknowledgement: %w", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			requestID, err := a.sendHeartbeatRequest(conn)
			if err != nil {
				return err
			}
			if err := a.waitHeartbeatAckIfRequired(ctx, interval, requestID); err != nil {
				return fmt.Errorf("wait for heartbeat acknowledgement: %w", err)
			}
		}
	}
}

func (a *Agent) waitHeartbeatAckIfRequired(ctx context.Context, interval time.Duration, requestID string) error {
	if !a.heartbeatAckRequired() {
		return nil
	}
	return waitHeartbeatAck(ctx, heartbeatAckTimeout(interval), requestID, a.heartbeatAckChannel())
}

func (a *Agent) heartbeatAckChannel() <-chan string {
	a.heartbeatMu.Lock()
	defer a.heartbeatMu.Unlock()
	return a.heartbeatAcks
}

func heartbeatAckTimeout(interval time.Duration) time.Duration {
	timeout := interval * 3
	if timeout < 15*time.Second {
		return 15 * time.Second
	}
	return timeout
}

func waitHeartbeatAck(ctx context.Context, timeout time.Duration, requestID string, acks <-chan string) error {
	if acks == nil {
		return nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case acknowledgedID := <-acks:
			if acknowledgedID == "" || acknowledgedID == requestID {
				return nil
			}
		case <-timer.C:
			return context.DeadlineExceeded
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (a *Agent) sendHeartbeat(conn controlConn) error {
	_, err := a.sendHeartbeatRequest(conn)
	return err
}

func (a *Agent) sendHeartbeatRequest(conn controlConn) (string, error) {
	requestID := fmt.Sprintf("heartbeat-%d", a.heartbeatSequence.Add(1))
	message, err := protocol.NewMessage(protocol.TypeHeartbeat, requestID, "", nil)
	if err != nil {
		return "", err
	}
	if queued, ok := conn.(*queuedControlConn); ok && queued.isClosed() {
		return "", errAgentControlWriteQueueClosed
	}
	if err := a.writeControl(conn, message); err != nil {
		return "", fmt.Errorf("send heartbeat: %w", err)
	}
	if pinger, ok := conn.(interface{ ping() error }); ok {
		if err := pinger.ping(); err != nil {
			return "", fmt.Errorf("send WebSocket ping: %w", err)
		}
	}
	return requestID, nil
}

func (a *Agent) writeControl(conn controlConn, message protocol.ControlMessage) error {
	return conn.writeJSON(message)
}

func (a *Agent) handleTunnelSnapshot(message protocol.ControlMessage) error {
	payload, err := protocol.DecodePayload[protocol.TunnelSnapshotPayload](message)
	if err != nil {
		return err
	}
	a.notifyTunnelSnapshot(payload.Tunnels)
	a.notifySessionReady()
	a.logger.Info("received tunnel snapshot", "count", len(payload.Tunnels))
	return nil
}

func (a *Agent) handleTunnelPath(message protocol.ControlMessage) error {
	payload, err := protocol.DecodePayload[protocol.TunnelPathPayload](message)
	if err != nil {
		return err
	}
	if message.TunnelID == "" || payload.Path == "" {
		return fmt.Errorf("tunnel path is missing tunnel_id or path")
	}
	a.notifyTunnelPath(message.TunnelID, payload.Path)
	a.logger.Debug("received tunnel path", "tunnel_id", message.TunnelID, "path", payload.Path)
	return nil
}

func sleep(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
