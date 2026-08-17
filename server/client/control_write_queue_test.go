package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

func TestControlWriteQueueSerializesAndDrainsMessages(t *testing.T) {
	transport := &recordingTransport{messages: make(chan protocol.ControlMessage, 256)}
	queue := newControlWriteQueue(transport, nil)
	defer queue.close()
	const count = 256
	for index := 0; index < count; index++ {
		if err := queue.enqueue(protocol.ControlMessage{RequestID: string(rune(index + 1))}); err != nil {
			t.Fatal(err)
		}
	}
	queue.mu.Lock()
	for queue.queuedCount > 0 {
		queue.condition.Wait()
	}
	queue.mu.Unlock()
	for index := 0; index < count; index++ {
		select {
		case <-transport.messages:
		case <-time.After(time.Second):
			t.Fatalf("message %d did not reach the transport", index)
		}
	}
}

func TestControlWriteQueueCloseStopsPendingWrites(t *testing.T) {
	transport := &recordingTransport{messages: make(chan protocol.ControlMessage, 1)}
	queue := newControlWriteQueue(transport, nil)
	if err := queue.enqueue(protocol.ControlMessage{}); err != nil {
		t.Fatal(err)
	}
	queue.close()
	if err := queue.enqueue(protocol.ControlMessage{}); !errors.Is(err, errControlWriteQueueClosed) {
		t.Fatalf("enqueue after close error=%v", err)
	}
}

func TestControlWriteQueueReportsTransportFailure(t *testing.T) {
	transport := &recordingTransport{err: errors.New("write failed")}
	failed := make(chan struct{})
	queue := newControlWriteQueue(transport, func() { close(failed) })
	if err := queue.enqueue(protocol.ControlMessage{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-failed:
	case <-time.After(time.Second):
		t.Fatal("transport failure was not reported")
	}
}

func TestControlWriteQueueWaitsForTransportWrite(t *testing.T) {
	transport := &recordingTransport{messages: make(chan protocol.ControlMessage, 1)}
	queue := newControlWriteQueue(transport, nil)
	defer queue.close()
	want := protocol.ControlMessage{Type: protocol.TypeOpenConnection, RequestID: "write-1"}
	if err := queue.enqueueWait(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-transport.messages:
		if got.RequestID != want.RequestID {
			t.Fatalf("request_id=%q, want %q", got.RequestID, want.RequestID)
		}
	case <-time.After(time.Second):
		t.Fatal("transport write did not complete")
	}
}

func TestControlWriteQueuePrioritizesHeartbeatOverConnectionBurst(t *testing.T) {
	transport := &blockingTransport{
		started: make(chan struct{}), release: make(chan struct{}),
		messages: make(chan protocol.ControlMessage, 16),
	}
	queue := newControlWriteQueue(transport, nil)
	defer queue.close()
	if err := queue.enqueue(protocol.ControlMessage{RequestID: "blocked"}); err != nil {
		t.Fatal(err)
	}
	<-transport.started
	for index := 0; index < 10; index++ {
		if err := queue.enqueue(protocol.ControlMessage{Type: protocol.TypeOpenConnection}); err != nil {
			t.Fatal(err)
		}
	}
	if err := queue.enqueue(protocol.ControlMessage{Type: protocol.TypeHeartbeatAck, RequestID: "heartbeat"}); err != nil {
		t.Fatal(err)
	}
	close(transport.release)
	<-transport.messages
	select {
	case message := <-transport.messages:
		if message.RequestID != "heartbeat" {
			t.Fatalf("second message=%+v, want heartbeat acknowledgement", message)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat acknowledgement was not written")
	}
}

type recordingTransport struct {
	mu       sync.Mutex
	messages chan protocol.ControlMessage
	err      error
}

func (t *recordingTransport) ReadJSON(any) error              { return nil }
func (t *recordingTransport) Close() error                    { return nil }
func (t *recordingTransport) SetReadDeadline(time.Time) error { return nil }
func (t *recordingTransport) WriteJSON(value any) error {
	if t.err != nil {
		return t.err
	}
	message, ok := value.(protocol.ControlMessage)
	if !ok {
		return errors.New("unexpected message type")
	}
	t.messages <- message
	return nil
}

type blockingTransport struct {
	once     sync.Once
	started  chan struct{}
	release  chan struct{}
	messages chan protocol.ControlMessage
}

func (*blockingTransport) ReadJSON(any) error              { return nil }
func (*blockingTransport) Close() error                    { return nil }
func (*blockingTransport) SetReadDeadline(time.Time) error { return nil }

func (t *blockingTransport) WriteJSON(value any) error {
	message := value.(protocol.ControlMessage)
	t.once.Do(func() {
		close(t.started)
		<-t.release
	})
	t.messages <- message
	return nil
}
