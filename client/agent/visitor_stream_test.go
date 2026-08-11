package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/nrytex/nrynet/internal/protocol"
)

func TestVisitorDataSessionBoundsAndCancelsRequests(t *testing.T) {
	agent := &Agent{}
	session := newVisitorDataSession(agent, context.Background())
	for i := 0; i < visitorMaxVisitorStreams; i++ {
		if err := session.start(visitorRequestStart(fmt.Sprintf("request-%d", i))); err != nil {
			t.Fatalf("start request %d: %v", i, err)
		}
	}
	if err := session.start(visitorRequestStart("overflow")); err == nil {
		t.Fatal("expected the per-session request limit to reject overflow")
	}
	if len(session.requests) != visitorMaxVisitorStreams {
		t.Fatalf("pending requests=%d want=%d", len(session.requests), visitorMaxVisitorStreams)
	}

	session.close()
	select {
	case <-session.ctx.Done():
	default:
		t.Fatal("session context was not canceled")
	}
	if len(session.slots) != 0 || len(session.globalSlots) != 0 {
		t.Fatalf("request slots were not released: local=%d global=%d", len(session.slots), len(session.globalSlots))
	}
}

func visitorRequestStart(id string) protocol.VisitorWebRTCDataMessage {
	return protocol.VisitorWebRTCDataMessage{Kind: "request_start", ID: id, Method: "GET", Path: "/"}
}
