package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"

	"github.com/nrytex/nrynet/internal/protocol"
)

const (
	visitorStreamChunkBytes  = 24 * 1024
	visitorMaxRequestBytes   = 16 * 1024 * 1024
	visitorMaxVisitorStreams = 64
)

type visitorDataSession struct {
	agent *Agent

	mu       sync.Mutex
	closed   bool
	requests map[string]*visitorStreamRequest
	slots    chan struct{}
	sendMu   sync.Mutex
}

type visitorStreamRequest struct {
	method  string
	path    string
	headers map[string][]string
	body    bytes.Buffer
}

func newVisitorDataSession(agent *Agent) *visitorDataSession {
	return &visitorDataSession{
		agent:    agent,
		requests: make(map[string]*visitorStreamRequest),
		slots:    make(chan struct{}, visitorMaxVisitorStreams),
	}
}

func (s *visitorDataSession) close() {
	s.mu.Lock()
	s.closed = true
	s.requests = make(map[string]*visitorStreamRequest)
	s.mu.Unlock()
}

func (s *visitorDataSession) handle(
	ctx context.Context,
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
		go s.agent.handleVisitorDataMessage(ctx, channel, localHost, localPort, data)
	case "request_start":
		s.start(channel, message)
	case "request_chunk":
		s.chunk(channel, message)
	case "request_end":
		s.end(ctx, channel, localHost, localPort, message)
	default:
		_ = s.sendError(channel, "unknown visitor request frame")
	}
}

func (s *visitorDataSession) start(channel *webrtc.DataChannel, message protocol.VisitorWebRTCDataMessage) {
	if message.ID == "" || message.Method == "" {
		_ = s.sendErrorForID(channel, message.ID, "request id and method are required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if len(s.requests) >= visitorMaxVisitorStreams {
		go s.sendErrorForID(channel, message.ID, "too many visitor requests")
		return
	}
	if _, exists := s.requests[message.ID]; exists {
		go s.sendErrorForID(channel, message.ID, "duplicate visitor request id")
		return
	}
	s.requests[message.ID] = &visitorStreamRequest{
		method:  message.Method,
		path:    message.Path,
		headers: cloneVisitorHeaders(message.Headers),
	}
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
	ctx context.Context,
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
	go func() {
		select {
		case s.slots <- struct{}{}:
			defer func() { <-s.slots }()
		case <-ctx.Done():
			_ = s.sendErrorForID(channel, message.ID, ctx.Err().Error())
			return
		}
		s.execute(ctx, channel, localHost, localPort, message.ID, request)
	}()
}

func (s *visitorDataSession) execute(
	ctx context.Context,
	channel *webrtc.DataChannel,
	localHost string,
	localPort int,
	id string,
	request *visitorStreamRequest,
) {
	target, err := visitorTargetURL(localHost, localPort, request.path)
	if err != nil {
		_ = s.sendErrorForID(channel, id, err.Error())
		return
	}
	httpRequest, err := http.NewRequestWithContext(ctx, request.method, target, bytes.NewReader(request.body.Bytes()))
	if err != nil {
		_ = s.sendErrorForID(channel, id, fmt.Errorf("invalid HTTP request: %w", err).Error())
		return
	}
	copyVisitorStreamHeaders(httpRequest.Header, request.headers)
	response, err := visitorHTTPClient().Do(httpRequest)
	if err != nil {
		_ = s.sendErrorForID(channel, id, fmt.Errorf("local service request failed: %w", err).Error())
		return
	}
	defer response.Body.Close()
	if err := s.sendFrame(channel, protocol.VisitorWebRTCDataMessage{
		Kind: "response_start", ID: id, Status: response.StatusCode,
		Headers: visitorResponseHeaders(response.Header),
	}); err != nil {
		return
	}
	buffer := make([]byte, visitorStreamChunkBytes)
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			if err := s.sendFrame(channel, protocol.VisitorWebRTCDataMessage{
				Kind: "response_chunk", ID: id,
				Body: base64.StdEncoding.EncodeToString(buffer[:count]),
			}); err != nil {
				return
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = s.sendErrorForID(channel, id, fmt.Errorf("read local service response: %w", readErr).Error())
			return
		}
	}
	_ = s.sendFrame(channel, protocol.VisitorWebRTCDataMessage{Kind: "response_end", ID: id})
}

func (s *visitorDataSession) sendError(channel *webrtc.DataChannel, message string) error {
	return s.sendErrorForID(channel, "", message)
}

func (s *visitorDataSession) sendErrorForID(channel *webrtc.DataChannel, id, message string) error {
	return s.sendFrame(channel, protocol.VisitorWebRTCDataMessage{Kind: "response_end", ID: id, Error: message})
}

func (s *visitorDataSession) removeAndError(channel *webrtc.DataChannel, id, message string) error {
	s.mu.Lock()
	delete(s.requests, id)
	s.mu.Unlock()
	return s.sendErrorForID(channel, id, message)
}

func (s *visitorDataSession) sendFrame(channel *webrtc.DataChannel, message protocol.VisitorWebRTCDataMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(data) > visitorMaxMessageBytes {
		return fmt.Errorf("visitor response frame is too large")
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return channel.SendText(string(data))
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
