package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

func TestOpenConnectionDispatchAcceptsMoreThan128Requests(t *testing.T) {
	agent := &Agent{logger: slog.Default()}
	control := &blockingOpenControl{
		release: make(chan struct{}),
		started: make(chan struct{}, 256),
		pending: &sync.WaitGroup{},
	}
	const requests = 256
	control.pending.Add(requests)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	acceptDone := make(chan struct{})
	go acceptConnections(listener, requests, acceptDone)
	message := openMessage(t, "request-0", listener.Addr().String())
	for index := 0; index < requests; index++ {
		message.RequestID = fmt.Sprintf("request-%d", index)
		if err := agent.handleControlMessage(context.Background(), control, message); err != nil {
			t.Fatalf("request %d was rejected: %v", index, err)
		}
	}
	waitForStartedOpenConnections(t, control.started, requests)
	close(control.release)
	waitForOpenConnections(t, control.pending)
	<-acceptDone
}

func acceptConnections(listener net.Listener, count int, done chan<- struct{}) {
	defer close(done)
	for range count {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}
}

func waitForStartedOpenConnections(t *testing.T, started <-chan struct{}, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d open connection workers reached the data channel", index)
		}
	}
}

func waitForOpenConnections(t *testing.T, pending *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		pending.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("open connection workers did not finish")
	}
}

type blockingOpenControl struct {
	release chan struct{}
	started chan struct{}
	pending *sync.WaitGroup
}

func (c *blockingOpenControl) readJSON(any) error  { return nil }
func (c *blockingOpenControl) writeJSON(any) error { return nil }
func (c *blockingOpenControl) close() error        { return nil }
func (c *blockingOpenControl) openData(ctx context.Context, _ string) (dataConn, error) {
	defer c.pending.Done()
	c.started <- struct{}{}
	select {
	case <-c.release:
		return &testDataConn{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
