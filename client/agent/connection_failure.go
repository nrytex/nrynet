package agent

import (
	"context"
	"fmt"

	"github.com/nrytex/nrynet/internal/protocol"
)

func (a *Agent) reportConnectionFailure(
	ctx context.Context,
	conn controlConn,
	message protocol.ControlMessage,
	cause error,
) {
	if cause == nil || message.RequestID == "" || ctx.Err() != nil {
		return
	}
	failure, err := protocol.NewMessage(
		protocol.TypeConnectionFailed,
		message.RequestID,
		message.TunnelID,
		protocol.ErrorPayload{Message: cause.Error()},
	)
	if err != nil {
		return
	}
	if err := a.writeControl(conn, failure); err != nil {
		a.logger.Debug("connection failure report failed", "request_id", message.RequestID, "error", fmt.Sprint(err))
	}
}
