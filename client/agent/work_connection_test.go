package agent

import (
	"encoding/json"
	"io"
	"net"
	"testing"

	"github.com/nrytex/nrynet/internal/protocol"
)

func TestWorkAssignmentPreservesCoalescedPayload(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	go func() {
		assignment := protocol.WorkConnectionAssignment{
			RequestID: "work-1", TunnelID: "tun-1", LocalHost: "127.0.0.1", LocalPort: 19000,
		}
		data, _ := json.Marshal(assignment)
		_, _ = left.Write(append(append(data, '\n'), []byte("payload")...))
	}()
	buffered := newBufferedDataConn(right)
	assignment, err := readWorkAssignment(buffered)
	if err != nil {
		t.Fatal(err)
	}
	if assignment.RequestID != "work-1" {
		t.Fatalf("assignment=%+v", assignment)
	}
	payload := make([]byte, len("payload"))
	if _, err := io.ReadFull(buffered, payload); err != nil {
		t.Fatal(err)
	}
	if string(payload) != "payload" {
		t.Fatalf("payload=%q", payload)
	}
}
