package relay

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/auth"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
	"github.com/nrytex/nrynet/internal/storage"
)

var (
	errDuplicateRequest = errors.New("request already pending")
	errRequestNotFound  = errors.New("request is not pending")
)

type CompleteFunc func(upload, download int64)

type Broker struct {
	auth    *auth.Service
	store   *storage.Store
	timeout time.Duration

	mu           sync.Mutex
	pending      map[string]*pendingConn
	connections  *clientConnections
	meter        *bandwidthMeter
	relayToken   string
	relayVisitor func(nodeID, tunnelID, visitorAddr string, visitor net.Conn) error
}

func (b *Broker) SetRelayVisitorHandler(token string, handler func(string, string, string, net.Conn) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.relayToken = token
	b.relayVisitor = handler
}

func NewBroker(authService *auth.Service, store *storage.Store, timeout time.Duration) *Broker {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Broker{
		auth:        authService,
		store:       store,
		timeout:     timeout,
		pending:     make(map[string]*pendingConn),
		connections: newClientConnections(),
		meter:       newBandwidthMeter(),
	}
}

func (b *Broker) BandwidthBPS() int64 {
	return b.meter.BytesPerSecond()
}

func (b *Broker) RecordBytes(bytes int64) {
	b.meter.Add(bytes)
}

func (b *Broker) Run(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go b.handleDataConn(conn)
	}
}

func (b *Broker) Pair(requestID string, visitor net.Conn, tunnel model.Tunnel, onComplete CompleteFunc) error {
	pending, err := b.RegisterPending(requestID, visitor, tunnel, onComplete)
	if err != nil {
		return err
	}
	return b.Wait(requestID, pending)
}

func (b *Broker) Wait(requestID string, pending *Pending) error {
	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	select {
	case err := <-pending.Done():
		return err
	case <-pending.Paired():
		return <-pending.Done()
	case <-timer.C:
		if !b.cancelPending(requestID, pending.entry) {
			return <-pending.Done()
		}
		_ = pending.entry.visitor.Close()
		return fmt.Errorf("wait for data channel: %w", context.DeadlineExceeded)
	}
}

func (b *Broker) Cancel(requestID string, pending *Pending) {
	if pending == nil {
		return
	}
	if b.cancelPending(requestID, pending.entry) {
		_ = pending.entry.visitor.Close()
	}
}

func (b *Broker) RegisterPending(
	requestID string,
	visitor net.Conn,
	tunnel model.Tunnel,
	onComplete CompleteFunc,
) (*Pending, error) {
	if requestID == "" {
		return nil, errors.New("request_id is required")
	}
	entry := &pendingConn{
		requestID:  requestID,
		visitor:    visitor,
		tunnel:     tunnel,
		onComplete: onComplete,
		done:       make(chan error, 1),
		paired:     make(chan struct{}),
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.pending[requestID]; exists {
		return nil, errDuplicateRequest
	}
	b.pending[requestID] = entry
	return &Pending{entry: entry}, nil
}

func (b *Broker) handleDataConn(conn net.Conn) {
	dataConn, initial, err := readInitialHandshake(conn)
	if err != nil {
		b.recordRejected("data handshake rejected", err)
		_ = conn.Close()
		return
	}
	if initial.Role == "relay_visitor" {
		b.handleRelayVisitor(dataConn, initial)
		return
	}
	handshake := protocol.DataHandshake{Token: initial.Token, DeviceID: initial.DeviceID, RequestID: initial.RequestID}
	pending, err := b.claimPending(handshake)
	if err != nil {
		b.recordRejected("agent data channel rejected", err)
		_ = dataConn.Close()
		return
	}
	err = b.relayPending(dataConn, pending)
	pending.done <- err
}

func (b *Broker) handleRelayVisitor(visitor net.Conn, handshake initialHandshake) {
	b.mu.Lock()
	token, handler := b.relayToken, b.relayVisitor
	b.mu.Unlock()
	if handler == nil || handshake.NodeID == "" || handshake.TunnelID == "" || handshake.VisitorAddr == "" ||
		token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(handshake.Token)) != 1 {
		_ = visitor.Close()
		return
	}
	if err := handler(handshake.NodeID, handshake.TunnelID, handshake.VisitorAddr, visitor); err != nil {
		b.recordRejected("relay visitor rejected", err)
		_ = visitor.Close()
	}
}

func (b *Broker) recordRejected(message string, err error) {
	_ = b.store.RecordEvent(context.Background(), "warn", "relay.rejected", message, map[string]any{
		"error": err.Error(),
	})
}

func (b *Broker) HandleAuthenticatedStream(stream dataStream, handshake protocol.DataHandshake, tokenID string) {
	pending, err := b.claimAuthenticatedPending(tokenID, handshake.DeviceID, handshake.RequestID)
	if err != nil {
		_ = stream.Close()
		return
	}
	err = b.relayPending(stream, pending)
	pending.done <- err
}

func (b *Broker) claimPending(handshake protocol.DataHandshake) (*pendingConn, error) {
	client, err := b.store.GetClientByDevice(context.Background(), handshake.DeviceID)
	if err != nil {
		return nil, err
	}
	generation := b.connections.generationFor(client.ID)
	token, err := b.auth.AuthenticateAgent(context.Background(), handshake.Token)
	if err != nil {
		return nil, err
	}
	if client.Disabled || client.TokenID != token.ID {
		return nil, errors.New("client is not authorized")
	}
	return b.claimPendingForClient(client.ID, handshake.RequestID, generation)
}

func (b *Broker) claimAuthenticatedPending(tokenID, deviceID, requestID string) (*pendingConn, error) {
	if tokenID == "" || deviceID == "" || requestID == "" {
		return nil, errors.New("token_id, device_id and request_id are required")
	}
	client, err := b.store.GetClientByDevice(context.Background(), deviceID)
	if err != nil {
		return nil, err
	}
	generation := b.connections.generationFor(client.ID)
	token, err := b.store.GetToken(context.Background(), tokenID)
	if err != nil {
		return nil, err
	}
	if client.Disabled || token.Disabled || client.TokenID != tokenID {
		return nil, errors.New("client is not authorized")
	}
	return b.claimPendingForClient(client.ID, requestID, generation)
}

func (b *Broker) claimPendingForClient(clientID, requestID string, generation uint64) (*pendingConn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := b.pending[requestID]
	if pending == nil {
		return nil, errRequestNotFound
	}
	if pending.tunnel.ClientID != clientID {
		return nil, errors.New("request does not belong to client")
	}
	pending.connectionGeneration = generation
	delete(b.pending, requestID)
	close(pending.paired)
	return pending, nil
}

func (b *Broker) cancelPending(requestID string, pending *pendingConn) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending[requestID] != pending {
		return false
	}
	delete(b.pending, requestID)
	return true
}

type Pending struct {
	entry *pendingConn
}

func (p *Pending) Done() <-chan error {
	return p.entry.done
}

func (p *Pending) Paired() <-chan struct{} {
	return p.entry.paired
}

type pendingConn struct {
	requestID            string
	visitor              net.Conn
	tunnel               model.Tunnel
	onComplete           CompleteFunc
	done                 chan error
	paired               chan struct{}
	connectionGeneration uint64
}
