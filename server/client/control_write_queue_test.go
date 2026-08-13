package client

import (
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
