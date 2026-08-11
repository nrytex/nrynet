package agent

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nrytex/nrynet/internal/protocol"
)

func TestExecuteVisitorRequestUsesLocalHTTPService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.RequestURI() != "/hello?name=nrynet" {
			t.Fatalf("request=%s %s", request.Method, request.URL.RequestURI())
		}
		if request.Header.Get("X-Visitor") != "p2p" {
			t.Fatalf("visitor header=%q", request.Header.Get("X-Visitor"))
		}
		writer.Header().Set("X-Reply", "agent")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte("accepted"))
	}))
	defer server.Close()

	host, port := splitTestAddress(t, server.Listener.Addr().String())
	response, err := executeVisitorRequest(context.Background(), host, port, protocol.VisitorWebRTCDataMessage{
		Kind: "request", ID: "1", Method: http.MethodPost, Path: "/hello?name=nrynet",
		Headers: map[string][]string{"X-Visitor": {"p2p"}},
		Body:    base64.StdEncoding.EncodeToString([]byte("request body")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated || response.Headers["X-Reply"][0] != "agent" {
		t.Fatalf("response=%+v", response)
	}
	body, err := base64.StdEncoding.DecodeString(response.Body)
	if err != nil || string(body) != "accepted" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestVisitorTargetURLRejectsExternalTargets(t *testing.T) {
	for _, path := range []string{"https://example.com/escape", "//example.com/escape", "relative"} {
		if _, err := visitorTargetURL("127.0.0.1", 8080, path); err == nil {
			t.Fatalf("path %q was accepted", path)
		}
	}
}

func TestDecodeVisitorBodyEnforcesLimit(t *testing.T) {
	value := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", visitorMaxBodyBytes+1)))
	if _, err := decodeVisitorBody(value); err == nil {
		t.Fatal("expected body limit error")
	}
}

func splitTestAddress(t *testing.T, address string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}
