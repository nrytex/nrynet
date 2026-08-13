package agent

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDialLocalWithRetryRecoversAfterTemporaryRefusal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	accepted := make(chan net.Conn, 1)
	listenErr := make(chan error, 1)
	go func() {
		time.Sleep(150 * time.Millisecond)
		listener, err := net.Listen("tcp", address)
		if err != nil {
			listenErr <- err
			return
		}
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil {
			listenErr <- err
			return
		}
		accepted <- conn
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialLocalWithRetry(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	select {
	case err := <-listenErr:
		t.Fatal(err)
	case acceptedConn := <-accepted:
		_ = acceptedConn.Close()
	case <-time.After(time.Second):
		t.Fatal("retry dial was not accepted")
	}
}

func TestDialLocalWithRetryStopsForMalformedAddress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	_, err := dialLocalWithRetry(ctx, "malformed-address")
	if err == nil {
		t.Fatal("expected refused local dial")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("non-retryable dial took too long: %v", time.Since(started))
	}
}
