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
	case protocol.TypeHeartbeatAck:
		a.enableHeartbeatAck()
		a.signalHeartbeatAck(message.RequestID)
		return nil
	case protocol.TypeOpenConnection:
		if !a.beginOpenRequest(message.RequestID) {
			a.logger.Debug("ignored duplicate open connection", "request_id", message.RequestID)
			return nil
		}
		a.goWorker("open connection", func() {
			defer a.endOpenRequest(message.RequestID)
			a.handleOpenConnection(ctx, conn, message)
		})
		return nil
	case protocol.TypeRequestWorkConn:
		if supportsWorkConnections(conn) {
			a.goWorker("work connection", func() {
				a.handleRequestedWorkConnection(ctx, conn)
			})
		}
		return nil
	case protocol.TypeUDPPacket:
		return a.handleUDPPacket(ctx, conn, message)
	case protocol.TypeP2PConnect:
		a.goWorker("p2p connection", func() {
			a.handleP2PConnect(a.relayContext(ctx), message)
		})
		return nil
	case protocol.TypeVisitorWebRTC:
		a.goWorker("visitor WebRTC", func() {
			_ = a.handleVisitorWebRTC(a.relayContext(ctx), conn, message)
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
