package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/config"
)

func TestDialControlWithFallsBackToWebSocketAfterQUICFailure(t *testing.T) {
	agent := &Agent{logger: slog.Default()}
	quicErr := errors.New("dial quic control session: timeout")
	websocketConn := &trackingControl{data: &testDataConn{}}
	var websocketCalls int

	got, err := agent.dialControlWith(
		context.Background(),
		func(context.Context) (controlConn, error) { return nil, quicErr },
		func(context.Context) (controlConn, error) {
			websocketCalls++
			return websocketConn, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != websocketConn || websocketCalls != 1 {
		t.Fatalf("control=%v websocket calls=%d", got, websocketCalls)
	}
	if !agent.webSocketFallbackEnabled() {
		t.Fatal("QUIC failure did not enable WebSocket fallback")
	}
}

func TestWebSocketControlUsesTCPDataHandshake(t *testing.T) {
	dataAddress, done := startDataPeer(t)
	agent := &Agent{
		options: Options{Config: config.ClientConfig{DataAddress: dataAddress, Token: "token-1", DeviceID: "device-1"}},
	}
	control := &websocketControl{agent: agent}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	data, err := control.openData(ctx, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	if !needsDataHandshake(data) {
		t.Fatal("WebSocket data channel did not require the TCP handshake")
	}
	if err := writeHandshake(data, agent.options.Config.Token, agent.options.Config.DeviceID, "req-1"); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(data, buffer); err != nil {
		t.Fatal(err)
	}
	if _, err := data.Write(buffer); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDialControlWithDoesNotFallbackAfterContextCancellation(t *testing.T) {
	agent := &Agent{logger: slog.Default()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var websocketCalls int
	_, err := agent.dialControlWith(
		ctx,
		func(context.Context) (controlConn, error) { return nil, context.Canceled },
		func(context.Context) (controlConn, error) {
			websocketCalls++
			return nil, errors.New("should not be called")
		},
	)
	if err == nil || websocketCalls != 0 {
		t.Fatalf("err=%v websocket calls=%d", err, websocketCalls)
	}
}

func TestMarkWebSocketFallbackOnlyAppliesToQUICControl(t *testing.T) {
	agent := &Agent{logger: slog.Default()}
	agent.markWebSocketFallback(&trackingControl{data: &testDataConn{}}, errors.New("websocket closed"))
	if agent.webSocketFallbackEnabled() {
		t.Fatal("WebSocket failure incorrectly disabled QUIC retries")
	}
}

func TestNormalizeOptionsFillsMissingTimingValues(t *testing.T) {
	options := normalizeOptions(Options{})
	if options.HeartbeatInterval <= 0 || options.ReconnectMin <= 0 || options.ReconnectMax < options.ReconnectMin {
		t.Fatalf("invalid normalized timing options: %+v", options)
	}
}
