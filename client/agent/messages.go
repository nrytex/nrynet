package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

func (a *Agent) heartbeat(ctx context.Context, conn controlConn) error {
	ticker := time.NewTicker(a.options.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			message, err := protocol.NewMessage(protocol.TypeHeartbeat, "", "", nil)
			if err != nil {
				return err
			}
			if err := conn.writeJSON(message); err != nil {
				return fmt.Errorf("send heartbeat: %w", err)
			}
		}
	}
}

func (a *Agent) handleTunnelSnapshot(message protocol.ControlMessage) error {
	payload, err := protocol.DecodePayload[protocol.TunnelSnapshotPayload](message)
	if err != nil {
		return err
	}
	a.notifyTunnelSnapshot(payload.Tunnels)
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
