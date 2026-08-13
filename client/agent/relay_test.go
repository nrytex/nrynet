package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/protocol"
)

func TestOpenAndRelayConnectsLocalServiceThroughDataChannel(t *testing.T) {
	localAddress := startLocalEcho(t)
	dataAddress, done := startDataPeer(t)
	options := Options{
		Config: config.ClientConfig{
			DataAddress: dataAddress,
			Token:       "token-1",
			DeviceID:    "device-1",
		},
	}
	agent := &Agent{options: options, logger: slog.Default()}
	message := openMessage(t, "req-1", localAddress)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := agent.openAndRelay(ctx, testControl{agent: agent}, message); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeCopyErrorTreatsPeerDisconnectAsNormal(t *testing.T) {
	for _, err := range expectedSocketCloseErrors() {
		if got := normalizeCopyError(err); got != nil {
			t.Fatalf("normalizeCopyError(%v)=%v", err, got)
		}
	}
	wrapped := &net.OpError{Op: "readfrom", Net: "tcp", Err: &os.SyscallError{Syscall: "wsasend", Err: expectedSocketCloseErrors()[0]}}
	if got := normalizeCopyError(wrapped); got != nil {
		t.Fatalf("normalizeCopyError(wrapped peer disconnect)=%v", got)
	}
}

func TestTransferWriterReportsWhileStreamIsOpen(t *testing.T) {
	var destination bytes.Buffer
	var reported int64
	writer := &transferWriter{
		writer:     &destination,
		observe:    func(size int64) { reported += size },
		lastReport: time.Now().Add(-time.Second),
	}
	if _, err := writer.Write(make([]byte, 64*1024)); err != nil {
		t.Fatal(err)
	}
	if reported != 64*1024 {
		t.Fatalf("reported=%d want=%d before stream close", reported, 64*1024)
	}
}

func TestOpenAndRelayFailsBeforeDataChannelWhenLocalServiceIsUnavailable(t *testing.T) {
	data := &testDataConn{}
	control := &trackingControl{data: data, opened: make(chan struct{})}
	agent := &Agent{
		options: Options{Config: config.ClientConfig{Transport: "quic"}},
		logger:  slog.Default(),
	}
	message := openMessage(t, "order-1", net.JoinHostPort("127.0.0.1", "1"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := agent.openAndRelay(ctx, control, message); err == nil {
		t.Fatal("expected local dial to fail")
	}
	select {
	case <-control.opened:
		t.Fatal("data channel opened after local service dial failed")
	default:
	}
}

func TestReportConnectionFailureSendsRequestID(t *testing.T) {
	control := &failureReportingControl{writes: make(chan protocol.ControlMessage, 1)}
	agent := &Agent{logger: slog.Default()}
	message := protocol.ControlMessage{Type: protocol.TypeOpenConnection, RequestID: "failed-1", TunnelID: "tun-1"}
	agent.reportConnectionFailure(context.Background(), control, message, errors.New("local service refused"))
	select {
	case got := <-control.writes:
		if got.Type != protocol.TypeConnectionFailed || got.RequestID != message.RequestID || got.TunnelID != message.TunnelID {
			t.Fatalf("unexpected failure message: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("failure message was not sent")
	}
}

func TestOpenConnectionDispatchReturnsWithoutWaitingForRelay(t *testing.T) {
	agent := &Agent{logger: slog.Default()}
	started := make(chan struct{})
	control := &trackingControl{data: &testDataConn{}, opened: started}
	message := protocol.ControlMessage{Type: protocol.TypeOpenConnection, RequestID: "capacity-1"}
	finished := make(chan struct{})
	go func() {
		_ = agent.handleControlMessage(context.Background(), control, message)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("open connection dispatch blocked on relay setup")
	}
}

func TestOpenQUICDataWithFallbackUsesLegacyRelay(t *testing.T) {
	legacy := &testDataConn{}
	data, err, usedFallback := openQUICDataWithFallback(
		context.Background(), "fallback-1",
		func(context.Context, string) (dataConn, error) { return nil, context.DeadlineExceeded },
		func(context.Context) (dataConn, error) { return legacy, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if data != legacy || !usedFallback {
		t.Fatalf("data=%v usedFallback=%v", data, usedFallback)
	}
}

func TestOpenQUICDataWithFallbackReportsBothErrors(t *testing.T) {
	_, err, usedFallback := openQUICDataWithFallback(
		context.Background(), "fallback-2",
		func(context.Context, string) (dataConn, error) { return nil, errors.New("quic unavailable") },
		func(context.Context) (dataConn, error) { return nil, errors.New("tcp unavailable") },
	)
	if err == nil || !strings.Contains(err.Error(), "quic unavailable") || !strings.Contains(err.Error(), "tcp unavailable") {
		t.Fatalf("unexpected error=%v", err)
	}
	if !usedFallback {
		t.Fatal("expected fallback attempt")
	}
}

type trackingControl struct {
	data   dataConn
	opened chan struct{}
}

type failureReportingControl struct {
	writes chan protocol.ControlMessage
}

func (c *failureReportingControl) readJSON(any) error { return nil }

func (c *failureReportingControl) writeJSON(value any) error {
	message, ok := value.(protocol.ControlMessage)
	if !ok {
		return fmt.Errorf("unexpected control message type %T", value)
	}
	c.writes <- message
	return nil
}

func (c *failureReportingControl) close() error { return nil }

func (c *failureReportingControl) openData(context.Context, string) (dataConn, error) {
	return nil, errors.New("not used")
}

func (c *trackingControl) readJSON(any) error  { return nil }
func (c *trackingControl) writeJSON(any) error { return nil }
func (c *trackingControl) close() error        { return nil }
func (c *trackingControl) openData(context.Context, string) (dataConn, error) {
	close(c.opened)
	return c.data, nil
}

type testDataConn struct{}

func (*testDataConn) Read([]byte) (int, error)       { return 0, io.EOF }
func (*testDataConn) Write(data []byte) (int, error) { return len(data), nil }
func (*testDataConn) Close() error                   { return nil }

type testControl struct {
	agent *Agent
}

func (c testControl) readJSON(any) error { return nil }

func (c testControl) writeJSON(any) error { return nil }

func (c testControl) close() error { return nil }

func (c testControl) openData(ctx context.Context, _ string) (dataConn, error) {
	return c.agent.dialLegacyData(ctx)
}

func startLocalEcho(t *testing.T) string {
	t.Helper()
	listener := listenLocal(t)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return listener.Addr().String()
}

func startDataPeer(t *testing.T) (string, <-chan error) {
	t.Helper()
	listener := listenLocal(t)
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		done <- exerciseDataConnection(conn)
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return listener.Addr().String(), done
}

func exerciseDataConnection(conn net.Conn) error {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	var handshake protocol.DataHandshake
	if err := json.NewDecoder(reader).Decode(&handshake); err != nil {
		return err
	}
	if handshake.Token != "token-1" || handshake.RequestID != "req-1" {
		return io.ErrUnexpectedEOF
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		return err
	}
	buffer := make([]byte, 4)
	_, err := io.ReadFull(reader, buffer)
	return err
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func openMessage(t *testing.T, requestID, address string) protocol.ControlMessage {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	message, err := protocol.NewMessage(protocol.TypeOpenConnection, requestID, "", protocol.OpenConnectionPayload{
		LocalHost: host,
		LocalPort: port,
	})
	if err != nil {
		t.Fatal(err)
	}
	return message
}
