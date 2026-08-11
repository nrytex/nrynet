package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"

	"github.com/nrytex/nrynet/internal/protocol"
)

const (
	visitorStreamChunkBytes  = 24 * 1024
	visitorMaxRequestBytes   = 16 * 1024 * 1024
	visitorMaxResponseBytes  = 16 * 1024 * 1024
	visitorMaxVisitorStreams = 32
	visitorMaxTotalStreams   = 64
)

type visitorDataSession struct {
	agent  *Agent
	ctx    context.Context
	cancel context.CancelFunc

	mu                sync.Mutex
	closed            bool
	requests          map[string]*visitorStreamRequest
	slots             chan struct{}
	globalSlots       chan struct{}
	sendMu            sync.Mutex
	bufferedAmountLow chan struct{}
}

type visitorStreamRequest struct {
	method  string
	path    string
	headers map[string][]string
	body    bytes.Buffer
}

func newVisitorDataSession(agent *Agent, parent context.Context) *visitorDataSession {
	ctx, cancel := context.WithCancel(parent)
	return &visitorDataSession{
		agent:             agent,
		ctx:               ctx,
		cancel:            cancel,
		requests:          make(map[string]*visitorStreamRequest),
		slots:             make(chan struct{}, visitorMaxVisitorStreams),
		globalSlots:       agent.visitorStreamSlots(),
		bufferedAmountLow: make(chan struct{}, 1),
	}
}

func (s *visitorDataSession) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	pending := len(s.requests)
	s.requests = make(map[string]*visitorStreamRequest)
	for range pending {
		<-s.slots
		<-s.globalSlots
	}
	cancel := s.cancel
	s.mu.Unlock()
	cancel()
}

func (s *visitorDataSession) handle(
	channel *webrtc.DataChannel,
	localHost string,
	localPort int,
	data []byte,
) {
	if len(data) > visitorMaxMessageBytes {
		_ = s.sendError(channel, "request frame is too large")
		return
	}
	var message protocol.VisitorWebRTCDataMessage
	if err := json.Unmarshal(data, &message); err != nil {
		_ = s.sendError(channel, "invalid visitor request frame")
		return
	}
	switch message.Kind {
	case "request":
		if !s.reserveSlot() {
			_ = s.sendErrorForID(channel, message.ID, "too many visitor requests")
			return
		}
		s.agent.goWorker("visitor request", func() {
			defer s.releaseSlot()
			s.agent.handleVisitorDataMessageWithSender(s.ctx, localHost, localPort, data, func(response protocol.VisitorWebRTCDataMessage) error {
				return s.sendFrame(channel, response)
			})
		})
	case "request_start":
		if err := s.start(message); err != nil {
			_ = s.sendErrorForID(channel, message.ID, err.Error())
		}
	case "request_chunk":
		s.chunk(channel, message)
	case "request_end":
		s.end(channel, localHost, localPort, message)
	default:
		_ = s.sendError(channel, "unknown visitor request frame")
	}
}

func (s *visitorDataSession) start(message protocol.VisitorWebRTCDataMessage) error {
	if message.ID == "" || message.Method == "" {
		return fmt.Errorf("request id and method are required")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return context.Canceled
	}
	if _, exists := s.requests[message.ID]; exists {
		s.mu.Unlock()
		return fmt.Errorf("duplicate visitor request id")
	}
	if !s.reserveSlotLocked() {
		s.mu.Unlock()
		return fmt.Errorf("too many visitor requests")
	}
	s.requests[message.ID] = &visitorStreamRequest{
		method:  message.Method,
		path:    message.Path,
		headers: cloneVisitorHeaders(message.Headers),
	}
	s.mu.Unlock()
	return nil
}

func (s *visitorDataSession) reserveSlot() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	return s.reserveSlotLocked()
}

func (s *visitorDataSession) reserveSlotLocked() bool {
	select {
	case s.slots <- struct{}{}:
	default:
		return false
	}
	select {
	case s.globalSlots <- struct{}{}:
		return true
	default:
		<-s.slots
		return false
	}
}

func (s *visitorDataSession) releaseSlot() {
	<-s.slots
	<-s.globalSlots
}

func (s *visitorDataSession) chunk(channel *webrtc.DataChannel, message protocol.VisitorWebRTCDataMessage) {
	chunk, err := base64.StdEncoding.DecodeString(message.Body)
	if err != nil {
		_ = s.removeAndError(channel, message.ID, "invalid request chunk")
		return
	}
	s.mu.Lock()
	request := s.requests[message.ID]
	tooLarge := request != nil && request.body.Len()+len(chunk) > visitorMaxRequestBytes
	if request != nil && !tooLarge {
		_, _ = request.body.Write(chunk)
	}
	s.mu.Unlock()
	if request == nil {
		_ = s.sendErrorForID(channel, message.ID, "visitor request was not started")
		return
	}
	if tooLarge {
		_ = s.removeAndError(channel, message.ID, fmt.Sprintf("request body exceeds %d bytes", visitorMaxRequestBytes))
	}
}

func (s *visitorDataSession) end(
	channel *webrtc.DataChannel,
	localHost string,
	localPort int,
	message protocol.VisitorWebRTCDataMessage,
) {
	s.mu.Lock()
	request := s.requests[message.ID]
	delete(s.requests, message.ID)
	s.mu.Unlock()
	if request == nil {
		_ = s.sendErrorForID(channel, message.ID, "visitor request was not started")
		return
	}
	s.agent.goWorker("visitor stream request", func() {
		defer s.releaseSlot()
		s.execute(channel, localHost, localPort, message.ID, request)
	})
}

func (s *visitorDataSession) sendError(channel *webrtc.DataChannel, message string) error {
	return s.sendErrorForID(channel, "", message)
}

func (s *visitorDataSession) sendErrorForID(channel *webrtc.DataChannel, id, message string) error {
	return s.sendFrame(channel, protocol.VisitorWebRTCDataMessage{Kind: "response_end", ID: id, Error: message})
}

func (s *visitorDataSession) removeAndError(channel *webrtc.DataChannel, id, message string) error {
	s.mu.Lock()
	if _, exists := s.requests[id]; exists {
		delete(s.requests, id)
		<-s.slots
		<-s.globalSlots
	}
	s.mu.Unlock()
	return s.sendErrorForID(channel, id, message)
}

func cloneVisitorHeaders(source map[string][]string) map[string][]string {
	result := make(map[string][]string, len(source))
	for name, values := range source {
		result[name] = append([]string(nil), values...)
	}
	return result
}

func copyVisitorStreamHeaders(destination http.Header, source map[string][]string) {
	for name, values := range source {
		if isVisitorHopByHopHeader(name) || strings.EqualFold(name, "Host") || strings.EqualFold(name, "Content-Length") || strings.EqualFold(name, "Accept-Encoding") {
			continue
		}
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

func visitorResponseHeaders(source http.Header) map[string][]string {
	result := make(map[string][]string, len(source))
	for name, values := range source {
		if isVisitorHopByHopHeader(name) || strings.EqualFold(name, "Content-Length") || strings.EqualFold(name, "Content-Encoding") {
			continue
		}
		result[name] = append([]string(nil), values...)
	}
	return result
}

func isVisitorHopByHopHeader(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
