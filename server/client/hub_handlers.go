package client

import (
	"encoding/json"

	"github.com/nrytex/nrynet/internal/protocol"
)

func (h *Hub) SetUDPPacketHandler(handler func(string, protocol.ControlMessage)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.udpHandler = handler
}

func (h *Hub) SetVisitorWebRTCHandler(handler func(string, protocol.ControlMessage)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.visitorWebRTCHandler = handler
}

func (h *Hub) SetConnectionFailureHandler(handler func(string, protocol.ControlMessage)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connectionFailureHandler = handler
}

func (h *Hub) SetConnectHandler(handler func(string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connectHandler = handler
}

func (h *Hub) SetDisconnectHandler(handler func(string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.disconnectHandler = handler
}

func (h *Hub) handleVisitorWebRTC(clientID string, message protocol.ControlMessage) {
	h.mu.RLock()
	handler := h.visitorWebRTCHandler
	h.mu.RUnlock()
	if handler != nil {
		handler(clientID, message)
	}
}

func (h *Hub) handleConnectionFailure(clientID string, message protocol.ControlMessage) {
	h.mu.RLock()
	handler := h.connectionFailureHandler
	h.mu.RUnlock()
	if handler != nil {
		handler(clientID, message)
	}
}

func errorMessage(text string) protocol.ControlMessage {
	payload, _ := json.Marshal(protocol.ErrorPayload{Message: text})
	return protocol.ControlMessage{Type: protocol.TypeError, Payload: payload}
}
