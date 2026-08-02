package protocol

import (
	"encoding/json"
	"fmt"
)

func NewMessage(messageType, requestID, tunnelID string, payload any) (ControlMessage, error) {
	data, err := marshalPayload(payload)
	if err != nil {
		return ControlMessage{}, err
	}
	return ControlMessage{
		Type:      messageType,
		RequestID: requestID,
		TunnelID:  tunnelID,
		Payload:   data,
	}, nil
}

func DecodePayload[T any](message ControlMessage) (T, error) {
	var payload T
	if len(message.Payload) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return payload, fmt.Errorf("decode %s payload: %w", message.Type, err)
	}
	return payload, nil
}

func marshalPayload(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	return data, nil
}
