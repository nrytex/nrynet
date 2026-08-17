package agent

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestRelaySurvivesControlSessionCancellation(t *testing.T) {
	agent := &Agent{logger: slog.Default()}
	runCtx, stopRun := context.WithCancel(context.Background())
	defer stopRun()
	agent.beginRun(runCtx)
	defer agent.endRun()
	sessionCtx, cancelSession := context.WithCancel(context.Background())
	defer cancelSession()

	data, dataPeer := net.Pipe()
	local, localPeer := net.Pipe()
	defer dataPeer.Close()
	defer localPeer.Close()
	done := make(chan error, 1)
	go func() { done <- agent.relay(agent.relayContext(sessionCtx), "long-lived", data, local) }()
	cancelSession()

	want := []byte("still connected")
	if _, err := dataPeer.Write(want); err != nil {
		t.Fatal(err)
	}
	if err := localPeer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(localPeer, got); err != nil {
		t.Fatalf("relay closed with control session: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("relayed payload=%q, want %q", got, want)
	}

	stopRun()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("relay did not stop with Agent context")
	}
}
