package client

import (
	"errors"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

func TestHubHeartbeatAckUsesTheCurrentTransport(t *testing.T) {
	store, authService := newHubStore(t)
	hub := NewHub(store, authService, time.Second)
	oldTransport := &recordingTransport{messages: make(chan protocol.ControlMessage, 1)}
	oldQueue := newControlWriteQueue(oldTransport, nil)
	hub.register("client-heartbeat", oldTransport, oldQueue)

	newTransport := &recordingTransport{messages: make(chan protocol.ControlMessage, 1)}
	newQueue := newControlWriteQueue(newTransport, nil)
	hub.register("client-heartbeat", newTransport, newQueue)
	defer hub.unregister("client-heartbeat", newTransport)

	if err := hub.sendHeartbeatAck("client-heartbeat", oldTransport, "heartbeat-old"); !errors.Is(err, errClientOffline) {
		t.Fatalf("old transport acknowledgement error = %v", err)
	}
	select {
	case message := <-newTransport.messages:
		t.Fatalf("stale transport sent an acknowledgement to replacement: %#v", message)
	case <-time.After(50 * time.Millisecond):
	}

	if err := hub.sendHeartbeatAck("client-heartbeat", newTransport, "heartbeat-current"); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-newTransport.messages:
		if message.Type != protocol.TypeHeartbeatAck || message.RequestID != "heartbeat-current" {
			t.Fatalf("unexpected acknowledgement: %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat acknowledgement did not reach the current transport")
	}
}

func TestHubHeartbeatAckFailureRemovesClient(t *testing.T) {
	store, authService := newHubStore(t)
	hub := NewHub(store, authService, time.Second)
	transport := &recordingTransport{err: errors.New("write failed")}
	queue := newControlWriteQueue(transport, nil)
	hub.register("client-heartbeat-failed", transport, queue)

	if err := hub.sendHeartbeatAck("client-heartbeat-failed", transport, "heartbeat-1"); err == nil {
		t.Fatal("heartbeat acknowledgement unexpectedly succeeded")
	}
	if hub.OnlineCount() != 0 {
		t.Fatalf("failed heartbeat left client online: %d", hub.OnlineCount())
	}
}
