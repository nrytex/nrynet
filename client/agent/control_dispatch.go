package agent

import (
	"context"
	"fmt"

	"github.com/nrytex/nrynet/internal/protocol"
)

func (a *Agent) handleControlMessage(
	ctx context.Context,
	conn controlConn,
	message protocol.ControlMessage,
) error {
	switch message.Type {
	case protocol.TypeTunnelSnapshot:
		return a.handleTunnelSnapshot(message)
	case protocol.TypeTunnelPath:
		return a.handleTunnelPath(message)
	case protocol.TypeOpenConnection:
		if !a.acquireStreamWorker(ctx) {
			return nil
		}
		a.goWorker("open connection", func() {
			defer a.releaseStreamWorker()
			a.handleOpenConnection(ctx, conn, message)
		})
		return nil
	case protocol.TypeUDPPacket:
		return a.handleUDPPacket(ctx, conn, message)
	case protocol.TypeP2PConnect:
		if !a.acquireStreamWorker(ctx) {
			return nil
		}
		a.goWorker("p2p connection", func() {
			defer a.releaseStreamWorker()
			a.handleP2PConnect(ctx, message)
		})
		return nil
	case protocol.TypeVisitorWebRTC:
		if !a.acquireVisitorSession() {
			a.logger.Warn("visitor WebRTC session capacity exhausted", "request_id", message.RequestID)
			return nil
		}
		a.goWorker("visitor WebRTC", func() {
			defer a.releaseVisitorSession()
			_ = a.handleVisitorWebRTC(ctx, conn, message)
		})
		return nil
	case protocol.TypeError:
		payload, err := protocol.DecodePayload[protocol.ErrorPayload](message)
		if err != nil {
			return err
		}
		return fmt.Errorf("server error: %s", payload.Message)
	default:
		a.logger.Debug("ignored control message", "type", message.Type)
		return nil
	}
}
