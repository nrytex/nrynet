package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

func TestQueuedControlConnSerializesWrites(t *testing.T) {
	base := &recordingAgentControl{messages: make(chan protocol.ControlMessage, 64)}
	queue := newQueuedControlConn(base)
	defer queue.close()
	for range 40 {
		if err := queue.writeJSON(protocol.ControlMessage{Type: protocol.TypeUDPPacket}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-base.messages:
	case <-time.After(time.Second):
		t.Fatal("queued message did not reach transport")
	}
}

func TestQueuedControlConnCloseRejectsNewWrites(t *testing.T) {
	base := &recordingAgentControl{}
	queue := newQueuedControlConn(base)
	if err := queue.close(); err != nil {
		t.Fatal(err)
	}
	if err := queue.writeJSON(protocol.ControlMessage{}); !errors.Is(err, errAgentControlWriteQueueClosed) {
		t.Fatalf("write after close error=%v", err)
	}
}

type recordingAgentControl struct {
	messages chan protocol.ControlMessage
}

func (c *recordingAgentControl) readJSON(any) error { return nil }
func (c *recordingAgentControl) close() error       { return nil }
func (c *recordingAgentControl) openData(context.Context, string) (dataConn, error) {
	return nil, errors.New("not used")
}
func (c *recordingAgentControl) writeJSON(value any) error {
	message, ok := value.(protocol.ControlMessage)
	if !ok {
		return errors.New("unexpected message type")
	}
	if c.messages != nil {
		c.messages <- message
	}
	return nil
}
