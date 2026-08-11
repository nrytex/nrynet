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

	"github.com/nrytex/nrynet/internal/protocol"
)

const (
	visitorMaxMessageBytes = 60 * 1024
	visitorMaxBodyBytes    = 40 * 1024
	visitorHTTPTimeout     = 15 * time.Second
)

func (a *Agent) handleVisitorDataMessageWithSender(
	ctx context.Context,
	localHost string,
	localPort int,
	data []byte,
	send visitorDataMessageSender,
) {
	if len(data) > visitorMaxMessageBytes {
		_ = send(protocol.VisitorWebRTCDataMessage{Kind: "response", Error: "request is too large"})
		return
	}
	var request protocol.VisitorWebRTCDataMessage
	if err := json.Unmarshal(data, &request); err != nil {
		_ = send(protocol.VisitorWebRTCDataMessage{Kind: "response", Error: "invalid request message"})
		return
	}
	if request.Kind != "request" || request.ID == "" {
		_ = send(protocol.VisitorWebRTCDataMessage{Kind: "response", ID: request.ID, Error: "request kind and id are required"})
		return
	}
	response, err := executeVisitorRequest(ctx, localHost, localPort, request)
	if err != nil {
		_ = send(protocol.VisitorWebRTCDataMessage{Kind: "response", ID: request.ID, Error: err.Error()})
		return
	}
	_ = send(protocol.VisitorWebRTCDataMessage{
		Kind: "response", ID: request.ID, Status: response.StatusCode,
		Headers: response.Headers, Body: response.Body,
	})
}

type visitorHTTPResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       string
}

type visitorDataMessageSender func(protocol.VisitorWebRTCDataMessage) error

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
	return visitorHTTPClientInstance
}

var visitorHTTPClientInstance = &http.Client{
	Timeout: visitorHTTPTimeout,
	Transport: &http.Transport{
		Proxy:                 nil,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   32,
		MaxConnsPerHost:       visitorMaxTotalStreams,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
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
