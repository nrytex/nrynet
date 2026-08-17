package relay

import (
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
)

const workConnectionSetupTimeout = 10 * time.Second

func (b *Broker) SetWorkConnectionHandlers(
	requestWorker func(string) error,
	openDataPath func(string, model.Tunnel, string) error,
) {
	b.mu.Lock()
	b.requestWorker = requestWorker
	b.openDataPath = openDataPath
	b.mu.Unlock()
}

func (b *Broker) PrimeWorkConnections(clientID string) {
	for range b.workers.target() {
		b.requestWorkConnection(clientID)
	}
}

func (b *Broker) CloseIdleWorkConnections() {
	b.workers.closeAll()
}

func (b *Broker) handleWorkConnection(conn net.Conn, initial initialHandshake) {
	transport, ok := conn.(workConnectionTransport)
	if !ok {
		_ = conn.Close()
		return
	}
	clientID, generation, err := b.authenticateDataClient(initial.Token, initial.DeviceID)
	if err != nil {
		b.recordRejected("work connection rejected", err)
		_ = conn.Close()
		return
	}
	worker := idleWorkConnection{conn: transport, clientID: clientID, generation: generation}
	if !b.workers.add(worker) {
		_ = conn.Close()
	}
}

func (b *Broker) TryWorkConnection(requestID string) bool {
	b.mu.Lock()
	pending := b.pending[requestID]
	b.mu.Unlock()
	if pending == nil {
		return false
	}
	worker, ok := b.workers.take(pending.tunnel.ClientID)
	if !ok {
		return false
	}
	go b.assignWorkConnection(worker, pending)
	return true
}

func (b *Broker) assignWorkConnection(worker idleWorkConnection, pending *pendingConn) {
	b.requestWorkConnection(worker.clientID)
	for {
		ready, err := startWorkConnection(worker.conn, pending)
		if err == nil {
			b.finishWorkConnection(worker, pending, ready)
			return
		}
		_ = worker.conn.Close()
		worker, _ = b.workers.take(worker.clientID)
		if worker.conn == nil {
			b.openFallbackDataPath(pending)
			return
		}
		b.requestWorkConnection(worker.clientID)
	}
}

func (b *Broker) finishWorkConnection(
	worker idleWorkConnection,
	pending *pendingConn,
	ready protocol.WorkConnectionReady,
) {
	if !ready.Ready {
		message := ready.Error
		if message == "" {
			message = "agent failed to open local service"
		}
		b.Fail(worker.clientID, pending.requestID, errors.New(message))
		_ = worker.conn.Close()
		return
	}
	claimed, err := b.claimPendingForClient(worker.clientID, pending.requestID, worker.generation)
	if err != nil {
		_ = worker.conn.Close()
		return
	}
	err = b.relayPending(worker.conn, claimed)
	claimed.done <- err
}

func startWorkConnection(
	conn workConnectionTransport,
	pending *pendingConn,
) (protocol.WorkConnectionReady, error) {
	assignment := protocol.WorkConnectionAssignment{
		RequestID: pending.requestID,
		TunnelID:  pending.tunnel.ID,
		LocalHost: pending.tunnel.LocalHost,
		LocalPort: pending.tunnel.LocalPort,
	}
	_ = conn.SetDeadline(time.Now().Add(workConnectionSetupTimeout))
	defer conn.SetDeadline(time.Time{})
	if err := json.NewEncoder(conn).Encode(assignment); err != nil {
		return protocol.WorkConnectionReady{}, err
	}
	var ready protocol.WorkConnectionReady
	if err := conn.readJSONLine(&ready); err != nil {
		return protocol.WorkConnectionReady{}, err
	}
	return ready, nil
}

func (b *Broker) requestWorkConnection(clientID string) {
	b.mu.Lock()
	handler := b.requestWorker
	b.mu.Unlock()
	if handler != nil {
		_ = handler(clientID)
	}
}

func (b *Broker) openFallbackDataPath(pending *pendingConn) {
	b.mu.Lock()
	handler := b.openDataPath
	current := b.pending[pending.requestID] == pending
	b.mu.Unlock()
	if !current || handler == nil {
		return
	}
	_ = handler(pending.tunnel.ClientID, pending.tunnel, pending.requestID)
}
