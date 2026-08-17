package relay

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
)

func TestBrokerUsesPreconnectedWorkConnection(t *testing.T) {
	store, authService, tokenValue, clientID := newBrokerStore(t)
	broker := NewBroker(authService, store, 2*time.Second)
	listener := listenLocal(t)
	go func() { _ = broker.Run(listener) }()

	worker := dialData(t, listener.Addr().String())
	defer worker.Close()
	writeWorkHandshake(t, worker, tokenValue, "device-1")
	waitForWorkConnection(t, broker, clientID)

	visitorPeer, visitorBroker := net.Pipe()
	defer visitorPeer.Close()
	pending, err := broker.RegisterPending("pooled-1", visitorBroker, model.Tunnel{
		ID: "tun-pooled", ClientID: clientID, LocalHost: "127.0.0.1", LocalPort: 19000,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	agentDone := runFakeWorkAgent(worker)
	if !broker.TryWorkConnection("pooled-1") {
		t.Fatal("expected a preconnected work connection")
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- broker.Wait("pooled-1", pending) }()

	if _, err := visitorPeer.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	assertRead(t, visitorPeer, "pong")
	_ = visitorPeer.Close()
	if err := <-agentDone; err != nil {
		t.Fatal(err)
	}
	if err := <-waitDone; err != nil {
		t.Fatal(err)
	}
}

func TestBrokerFallsBackWhenPooledConnectionIsStale(t *testing.T) {
	store, authService, _, clientID := newBrokerStore(t)
	broker := NewBroker(authService, store, time.Second)
	server, agent := net.Pipe()
	transport := bufferedConn{Conn: server, reader: bufio.NewReader(server)}
	broker.workers.add(idleWorkConnection{conn: transport, clientID: clientID})
	_ = agent.Close()

	visitorPeer, visitorBroker := net.Pipe()
	defer visitorPeer.Close()
	pending, err := broker.RegisterPending("stale-worker", visitorBroker, model.Tunnel{
		ID: "tun-stale", ClientID: clientID, LocalHost: "127.0.0.1", LocalPort: 19000,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fallback := make(chan string, 1)
	broker.SetWorkConnectionHandlers(nil, func(_ string, _ model.Tunnel, requestID string) error {
		fallback <- requestID
		return nil
	})
	if !broker.TryWorkConnection("stale-worker") {
		t.Fatal("expected stale worker to be selected before fallback")
	}
	select {
	case requestID := <-fallback:
		if requestID != "stale-worker" {
			t.Fatalf("fallback request=%q", requestID)
		}
	case <-time.After(time.Second):
		t.Fatal("stale worker did not trigger the normal data path")
	}
	broker.Cancel("stale-worker", pending)
}

func writeWorkHandshake(t *testing.T, conn net.Conn, token, deviceID string) {
	t.Helper()
	handshake := protocol.DataHandshake{
		Token: token, DeviceID: deviceID, Role: protocol.DataRoleWorkConnection,
	}
	if err := json.NewEncoder(conn).Encode(handshake); err != nil {
		t.Fatal(err)
	}
}

func waitForWorkConnection(t *testing.T, broker *Broker, clientID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for broker.workers.count(clientID) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("work connection was not registered")
		}
		time.Sleep(time.Millisecond)
	}
}

func runFakeWorkAgent(conn net.Conn) <-chan error {
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(conn)
		line, err := reader.ReadSlice('\n')
		if err != nil {
			done <- err
			return
		}
		var assignment protocol.WorkConnectionAssignment
		if err := json.Unmarshal(line, &assignment); err != nil {
			done <- err
			return
		}
		if err := json.NewEncoder(conn).Encode(protocol.WorkConnectionReady{Ready: true}); err != nil {
			done <- err
			return
		}
		buffer := make([]byte, 4)
		if _, err := io.ReadFull(reader, buffer); err != nil {
			done <- err
			return
		}
		_, err = conn.Write([]byte("pong"))
		_ = conn.Close()
		done <- err
	}()
	return done
}
