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
		a.goWorker("open connection", func() {
			a.handleOpenConnection(ctx, conn, message)
		})
		return nil
	case protocol.TypeUDPPacket:
		return a.handleUDPPacket(ctx, conn, message)
	case protocol.TypeP2PConnect:
		a.goWorker("p2p connection", func() {
			a.handleP2PConnect(ctx, message)
		})
		return nil
	case protocol.TypeVisitorWebRTC:
		a.goWorker("visitor WebRTC", func() {
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
