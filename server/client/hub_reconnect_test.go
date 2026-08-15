package client

import (
	"errors"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
)

func TestHubOpenConnectionWaitsForAgentReconnect(t *testing.T) {
	store, authService := newHubStore(t)
	hub := NewHub(store, authService, time.Second)
	transport := &recordingTransport{messages: make(chan protocol.ControlMessage, 1)}
	result := make(chan error, 1)
	tunnel := model.Tunnel{ID: "tunnel-1", LocalHost: "127.0.0.1", LocalPort: 19000}

	go func() { result <- hub.OpenConnection("client-1", tunnel, "request-1") }()
	time.Sleep(50 * time.Millisecond)
	queue := newControlWriteQueue(transport, nil)
	hub.register("client-1", transport, queue)
	defer hub.unregister("client-1", transport)

	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("OpenConnection did not resume after Agent reconnect")
	}
	select {
	case message := <-transport.messages:
		if message.Type != protocol.TypeOpenConnection || message.RequestID != "request-1" {
			t.Fatalf("unexpected message: %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("reconnected Agent did not receive OpenConnection")
	}
}

func TestHubQueuePressureDoesNotDisconnectAgent(t *testing.T) {
	store, authService := newHubStore(t)
	hub := NewHub(store, authService, time.Second)
	transport := &recordingTransport{messages: make(chan protocol.ControlMessage, 1)}
	queue := newControlWriteQueue(transport, nil)
	hub.register("client-2", transport, queue)
	defer hub.unregister("client-2", transport)

	queue.mu.Lock()
	queue.queuedCount = maxControlQueueMessages
	queue.mu.Unlock()
	err := hub.send("client-2", protocol.ControlMessage{Type: protocol.TypeOpenConnection})
	if !errors.Is(err, errControlWriteQueueFull) {
		t.Fatalf("send error=%v, want queue full", err)
	}
	if hub.OnlineCount() != 1 {
		t.Fatal("queue pressure disconnected the Agent")
	}
}
