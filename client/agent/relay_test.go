package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/internal/protocol"
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
	if err := agent.openAndRelay(ctx, message); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
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
