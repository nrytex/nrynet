package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/protocol"
)

func TestVisitorWebRTCBridgesDataChannelToLocalHTTP(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/hello" {
			t.Fatalf("path=%q", request.URL.Path)
		}
		_, _ = writer.Write([]byte("hello from agent"))
	}))
	defer local.Close()
	host, port := splitTestAddress(t, local.Listener.Addr().String())

	browser, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	channel, err := browser.CreateDataChannel("nrynet-visitor", nil)
	if err != nil {
		t.Fatal(err)
	}
	opened := make(chan struct{})
	channel.OnOpen(func() { close(opened) })
	responses := make(chan protocol.VisitorWebRTCDataMessage, 1)
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		var response protocol.VisitorWebRTCDataMessage
		if err := json.Unmarshal(message.Data, &response); err == nil {
			responses <- response
		}
	})
	offer, err := browser.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := browser.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	if err := waitForGathering(browser); err != nil {
		t.Fatal(err)
	}

	control := &visitorTestControl{writes: make(chan protocol.ControlMessage, 1)}
	agent := &Agent{options: Options{Config: config.ClientConfig{}}, logger: slog.Default()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	offerMessage, err := protocol.NewMessage(protocol.TypeVisitorWebRTC, "request-1", "tunnel-1", protocol.VisitorWebRTCSignalPayload{
		Kind: "offer", SessionID: "session-1", SDP: browser.LocalDescription().SDP,
		LocalHost: host, LocalPort: port,
	})
	if err != nil {
		t.Fatal(err)
	}
	agentDone := make(chan error, 1)
	go func() { agentDone <- agent.handleVisitorWebRTC(ctx, control, offerMessage) }()

	answerMessage := waitControlMessage(t, control.writes)
	answer, err := protocol.DecodePayload[protocol.VisitorWebRTCSignalPayload](answerMessage)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Kind != "answer" {
		t.Fatalf("signal kind=%q error=%q", answer.Kind, answer.Error)
	}
	if err := browser.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer.SDP}); err != nil {
		t.Fatal(err)
	}
	waitChannelOpen(t, opened)

	request, _ := json.Marshal(protocol.VisitorWebRTCDataMessage{Kind: "request", ID: "1", Method: http.MethodGet, Path: "/hello"})
	if err := channel.SendText(string(request)); err != nil {
		t.Fatal(err)
	}
	response := waitVisitorResponse(t, responses)
	if response.Error != "" || response.Status != http.StatusOK {
		t.Fatalf("response=%+v", response)
	}
	body, err := base64.StdEncoding.DecodeString(response.Body)
	if err != nil || string(body) != "hello from agent" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	_ = channel.Close()
	select {
	case err := <-agentDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent WebRTC session did not close")
	}
}

type visitorTestControl struct {
	writes chan protocol.ControlMessage
}

func (c *visitorTestControl) readJSON(any) error { return errors.New("not used") }

func (c *visitorTestControl) writeJSON(value any) error {
	message, ok := value.(protocol.ControlMessage)
	if !ok {
		return errors.New("unexpected control message type")
	}
	c.writes <- message
	return nil
}

func (c *visitorTestControl) close() error { return nil }

func (c *visitorTestControl) openData(context.Context, string) (dataConn, error) {
	return nil, errors.New("not used")
}

func waitForGathering(peer *webrtc.PeerConnection) error {
	complete := webrtc.GatheringCompletePromise(peer)
	select {
	case <-complete:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("test ICE gathering timed out")
	}
}

func waitControlMessage(t *testing.T, messages <-chan protocol.ControlMessage) protocol.ControlMessage {
	t.Helper()
	select {
	case message := <-messages:
		return message
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for WebRTC answer")
		return protocol.ControlMessage{}
	}
}

func waitChannelOpen(t *testing.T, opened <-chan struct{}) {
	t.Helper()
	select {
	case <-opened:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for WebRTC data channel")
	}
}

func waitVisitorResponse(t *testing.T, responses <-chan protocol.VisitorWebRTCDataMessage) protocol.VisitorWebRTCDataMessage {
	t.Helper()
	select {
	case response := <-responses:
		return response
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for WebRTC response")
		return protocol.VisitorWebRTCDataMessage{}
	}
}
