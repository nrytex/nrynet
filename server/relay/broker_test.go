package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/protocol"
	"github.com/nat-link/nat-link/internal/storage"
)

func TestBrokerPairsDataChannelWithVisitor(t *testing.T) {
	store, authService, tokenValue, clientID := newBrokerStore(t)
	tunnel := model.Tunnel{ID: "tun-1", ClientID: clientID}
	broker := NewBroker(authService, store, 2*time.Second)
	listener := listenLocal(t)
	go func() { _ = broker.Run(listener) }()

	visitorPeer, visitorBroker := net.Pipe()
	defer visitorPeer.Close()
	done := make(chan error, 1)
	var upload, download int64
	go func() {
		done <- broker.Pair("req-1", visitorBroker, tunnel, func(up, down int64) {
			upload, download = up, down
		})
	}()

	dataConn := dialData(t, listener.Addr().String())
	defer dataConn.Close()
	writeDataHandshake(t, dataConn, tokenValue, "device-1", "req-1")
	if _, err := visitorPeer.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	assertRead(t, dataConn, "ping")
	if _, err := dataConn.Write([]byte("pong")); err != nil {
		t.Fatal(err)
	}
	assertRead(t, visitorPeer, "pong")
	_ = dataConn.Close()
	_ = visitorPeer.Close()

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if upload != 4 || download != 4 {
		t.Fatalf("unexpected traffic counts: upload=%d download=%d", upload, download)
	}
}

func TestBrokerRejectsWrongDeviceForPendingTunnel(t *testing.T) {
	store, authService, tokenValue, clientID := newBrokerStore(t)
	other := addOnlineClient(t, store, authService, "device-2")
	broker := NewBroker(authService, store, 200*time.Millisecond)
	listener := listenLocal(t)
	go func() { _ = broker.Run(listener) }()

	_, visitorBroker := net.Pipe()
	defer visitorBroker.Close()
	done := make(chan error, 1)
	go func() {
		done <- broker.Pair("req-2", visitorBroker, model.Tunnel{ID: "tun-2", ClientID: clientID}, nil)
	}()
	dataConn := dialData(t, listener.Addr().String())
	writeDataHandshake(t, dataConn, tokenValue, other.DeviceID, "req-2")
	_ = dataConn.Close()

	err := <-done
	if err == nil {
		t.Fatal("expected pair timeout after unauthorized data channel")
	}
}

func newBrokerStore(t *testing.T) (*storage.Store, *auth.Service, string, string) {
	t.Helper()
	store, err := storage.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	authService, err := auth.New(context.Background(), store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, value, err := authService.CreateAgentToken(context.Background(), "agent")
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "agent", DeviceID: "device-1", IP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, authService, value, client.ID
}

func addOnlineClient(t *testing.T, store *storage.Store, authService *auth.Service, deviceID string) model.Client {
	t.Helper()
	token, _, err := authService.CreateAgentToken(context.Background(), deviceID)
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: deviceID, DeviceID: deviceID, IP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func dialData(t *testing.T, address string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func writeDataHandshake(t *testing.T, conn net.Conn, token, deviceID, requestID string) {
	t.Helper()
	writer := bufio.NewWriter(conn)
	handshake := protocol.DataHandshake{Token: token, DeviceID: deviceID, RequestID: requestID}
	if err := json.NewEncoder(writer).Encode(handshake); err != nil {
		t.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
}

func assertRead(t *testing.T, reader io.Reader, want string) {
	t.Helper()
	buffer := make([]byte, len(want))
	if _, err := io.ReadFull(reader, buffer); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != want {
		t.Fatalf("want %q, got %q", want, string(buffer))
	}
}
