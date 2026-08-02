package relay

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/protocol"
	"github.com/nat-link/nat-link/internal/storage"
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
		auth:    authService,
		store:   store,
		timeout: timeout,
		pending: make(map[string]*pendingConn),
		meter:   newBandwidthMeter(),
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
	err = b.relay(dataConn, pending.visitor, pending.onComplete)
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

func (b *Broker) HandleAuthenticatedStream(stream dataStream, handshake protocol.DataHandshake) {
	pending, err := b.claimAuthenticatedPending(handshake.DeviceID, handshake.RequestID)
	if err != nil {
		_ = stream.Close()
		return
	}
	err = b.relay(stream, pending.visitor, pending.onComplete)
	pending.done <- err
}

func (b *Broker) claimPending(handshake protocol.DataHandshake) (*pendingConn, error) {
	token, err := b.auth.AuthenticateAgent(context.Background(), handshake.Token)
	if err != nil {
		return nil, err
	}
	client, err := b.store.GetClientByDevice(context.Background(), handshake.DeviceID)
	if err != nil {
		return nil, err
	}
	if client.Disabled || client.TokenID != token.ID {
		return nil, errors.New("client is not authorized")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := b.pending[handshake.RequestID]
	if pending == nil {
		return nil, errRequestNotFound
	}
	if pending.tunnel.ClientID != client.ID {
		return nil, errors.New("request does not belong to client")
	}
	delete(b.pending, handshake.RequestID)
	close(pending.paired)
	return pending, nil
}

func (b *Broker) claimAuthenticatedPending(deviceID, requestID string) (*pendingConn, error) {
	if deviceID == "" || requestID == "" {
		return nil, errors.New("device_id and request_id are required")
	}
	client, err := b.store.GetClientByDevice(context.Background(), deviceID)
	if err != nil {
		return nil, err
	}
	if client.Disabled {
		return nil, errors.New("client is not authorized")
	}
	return b.claimPendingForClient(client.ID, requestID)
}

func (b *Broker) claimPendingForClient(clientID, requestID string) (*pendingConn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := b.pending[requestID]
	if pending == nil {
		return nil, errRequestNotFound
	}
	if pending.tunnel.ClientID != clientID {
		return nil, errors.New("request does not belong to client")
	}
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
	requestID  string
	visitor    net.Conn
	tunnel     model.Tunnel
	onComplete CompleteFunc
	done       chan error
	paired     chan struct{}
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

type initialHandshake struct {
	Role        string `json:"role"`
	Token       string `json:"token"`
	DeviceID    string `json:"device_id"`
	RequestID   string `json:"request_id"`
	NodeID      string `json:"node_id"`
	TunnelID    string `json:"tunnel_id"`
	VisitorAddr string `json:"visitor_addr"`
}

func readInitialHandshake(conn net.Conn) (net.Conn, initialHandshake, error) {
	reader := bufio.NewReaderSize(conn, 4096)
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return nil, initialHandshake{}, err
	}
	var handshake initialHandshake
	if err := json.Unmarshal(line, &handshake); err != nil {
		return nil, initialHandshake{}, err
	}
	if handshake.Role == "relay_visitor" {
		return bufferedConn{Conn: conn, reader: reader}, handshake, nil
	}
	if handshake.Token == "" || handshake.DeviceID == "" || handshake.RequestID == "" {
		return nil, initialHandshake{}, errors.New("invalid data handshake")
	}
	return bufferedConn{Conn: conn, reader: reader}, handshake, nil
}

func (c bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
