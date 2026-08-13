package tunnel

import (
	"errors"

	"github.com/nrytex/nrynet/internal/protocol"
)

func (m *Manager) HandleConnectionFailure(clientID string, message protocol.ControlMessage) {
	payload, err := protocol.DecodePayload[protocol.ErrorPayload](message)
	if err != nil {
		return
	}
	if payload.Message == "" {
		payload.Message = "agent failed to open the local service"
	}
	m.broker.Fail(clientID, message.RequestID, errors.New(payload.Message))
}
