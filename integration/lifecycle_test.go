package integration

import (
	"context"
	"net"
	"testing"
	"time"

	clientagent "github.com/nat-link/nat-link/client/agent"
	"github.com/nat-link/nat-link/server/relay"
)

func runBroker(t *testing.T, listener net.Listener, broker *relay.Broker) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = broker.Run(listener)
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		waitForShutdown(t, done, "relay broker")
	})
}

func runAgent(t *testing.T, ctx context.Context, cancel context.CancelFunc, agent *clientagent.Agent) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = agent.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		waitForShutdown(t, done, "client agent")
	})
}

func waitForShutdown(t *testing.T, done <-chan struct{}, component string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Errorf("%s did not stop before test cleanup", component)
	}
}
