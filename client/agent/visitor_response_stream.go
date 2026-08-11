package agent

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/pion/webrtc/v4"

	"github.com/nrytex/nrynet/internal/protocol"
)

func (s *visitorDataSession) execute(
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
	httpRequest, err := http.NewRequestWithContext(s.ctx, request.method, target, bytes.NewReader(request.body.Bytes()))
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
	if !s.streamResponseBody(channel, response.Body, id) {
		return
	}
	_ = s.sendFrame(channel, protocol.VisitorWebRTCDataMessage{Kind: "response_end", ID: id})
}

func (s *visitorDataSession) streamResponseBody(channel *webrtc.DataChannel, body io.Reader, id string) bool {
	buffer := make([]byte, visitorStreamChunkBytes)
	var responseBytes int64
	for {
		count, readErr := body.Read(buffer)
		if count > 0 {
			responseBytes += int64(count)
			if responseBytes > visitorMaxResponseBytes {
				_ = s.sendErrorForID(channel, id, fmt.Sprintf("response body exceeds %d bytes", visitorMaxResponseBytes))
				return false
			}
			if err := s.sendFrame(channel, protocol.VisitorWebRTCDataMessage{
				Kind: "response_chunk", ID: id,
				Body: base64.StdEncoding.EncodeToString(buffer[:count]),
			}); err != nil {
				return false
			}
		}
		if errors.Is(readErr, io.EOF) {
			return true
		}
		if readErr != nil {
			_ = s.sendErrorForID(channel, id, fmt.Errorf("read local service response: %w", readErr).Error())
			return false
		}
	}
}
