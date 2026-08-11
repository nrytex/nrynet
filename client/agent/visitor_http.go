package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/nrytex/nrynet/internal/protocol"
)

const (
	visitorMaxMessageBytes = 60 * 1024
	visitorMaxBodyBytes    = 40 * 1024
	visitorHTTPTimeout     = 15 * time.Second
)

func (a *Agent) handleVisitorDataMessage(
	ctx context.Context,
	channel *webrtc.DataChannel,
	localHost string,
	localPort int,
	data []byte,
) {
	if len(data) > visitorMaxMessageBytes {
		_ = sendVisitorError(channel, "request is too large")
		return
	}
	var request protocol.VisitorWebRTCDataMessage
	if err := json.Unmarshal(data, &request); err != nil {
		_ = sendVisitorError(channel, "invalid request message")
		return
	}
	if request.Kind != "request" || request.ID == "" {
		_ = sendVisitorResponse(channel, request.ID, 0, nil, "request kind and id are required", "")
		return
	}
	response, err := executeVisitorRequest(ctx, localHost, localPort, request)
	if err != nil {
		_ = sendVisitorResponse(channel, request.ID, 0, nil, err.Error(), "")
		return
	}
	_ = sendVisitorResponse(channel, request.ID, response.StatusCode, response.Headers, "", response.Body)
}

type visitorHTTPResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       string
}

func executeVisitorRequest(
	ctx context.Context,
	localHost string,
	localPort int,
	message protocol.VisitorWebRTCDataMessage,
) (visitorHTTPResponse, error) {
	body, err := decodeVisitorBody(message.Body)
	if err != nil {
		return visitorHTTPResponse{}, err
	}
	target, err := visitorTargetURL(localHost, localPort, message.Path)
	if err != nil {
		return visitorHTTPResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, message.Method, target, bytes.NewReader(body))
	if err != nil {
		return visitorHTTPResponse{}, fmt.Errorf("invalid HTTP request: %w", err)
	}
	copyVisitorHeaders(request.Header, message.Headers)
	response, err := visitorHTTPClient().Do(request)
	if err != nil {
		return visitorHTTPResponse{}, fmt.Errorf("local service request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := readVisitorBody(response.Body)
	if err != nil {
		return visitorHTTPResponse{}, err
	}
	return visitorHTTPResponse{
		StatusCode: response.StatusCode,
		Headers:    response.Header,
		Body:       base64.StdEncoding.EncodeToString(responseBody),
	}, nil
}

func decodeVisitorBody(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	body, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}
	if len(body) > visitorMaxBodyBytes {
		return nil, fmt.Errorf("request body exceeds %d bytes", visitorMaxBodyBytes)
	}
	return body, nil
}

func visitorTargetURL(host string, port int, path string) (string, error) {
	if host == "" || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid local service address")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("path must be an absolute local path")
	}
	parsed, err := url.ParseRequestURI(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "", fmt.Errorf("invalid request path")
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + path, nil
}

func copyVisitorHeaders(destination http.Header, source map[string][]string) {
	for name, values := range source {
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

func visitorHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   visitorHTTPTimeout,
		Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func readVisitorBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, visitorMaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read local service response: %w", err)
	}
	if len(body) > visitorMaxBodyBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", visitorMaxBodyBytes)
	}
	return body, nil
}

func sendVisitorResponse(
	channel *webrtc.DataChannel,
	id string,
	status int,
	headers map[string][]string,
	errText string,
	body string,
) error {
	message := protocol.VisitorWebRTCDataMessage{
		Kind: "response", ID: id, Status: status, Headers: headers, Error: errText, Body: body,
	}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(data) > visitorMaxMessageBytes {
		data, err = json.Marshal(protocol.VisitorWebRTCDataMessage{
			Kind: "response", ID: id, Error: "response is too large for the WebRTC data channel",
		})
		if err != nil {
			return err
		}
	}
	return channel.SendText(string(data))
}

func sendVisitorError(channel *webrtc.DataChannel, message string) error {
	return sendVisitorResponse(channel, "", 0, nil, message, "")
}
