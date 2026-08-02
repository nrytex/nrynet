package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	mu      sync.Mutex
	pending map[string]*pendingConn
	meter   *bandwidthMeter
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
	select {
	case err := <-pending.Done():
		return err
	case <-time.After(b.timeout):
		b.Cancel(requestID, pending)
		return fmt.Errorf("wait for data channel: %w", context.DeadlineExceeded)
	}
}

func (b *Broker) Cancel(requestID string, pending *Pending) {
	if pending == nil {
		return
	}
	b.removePending(requestID, pending.entry)
	_ = pending.entry.visitor.Close()
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
	dataConn, handshake, err := readHandshake(conn)
	if err != nil {
		_ = conn.Close()
		return
	}
	pending, err := b.claimPending(handshake)
	if err != nil {
		_ = dataConn.Close()
		return
	}
	err = b.relay(dataConn, pending.visitor, pending.onComplete)
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
	return pending, nil
}

func (b *Broker) removePending(requestID string, pending *pendingConn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending[requestID] == pending {
		delete(b.pending, requestID)
	}
}

type Pending struct {
	entry *pendingConn
}

func (p *Pending) Done() <-chan error {
	return p.entry.done
}

type pendingConn struct {
	requestID  string
	visitor    net.Conn
	tunnel     model.Tunnel
	onComplete CompleteFunc
	done       chan error
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func readHandshake(conn net.Conn) (net.Conn, protocol.DataHandshake, error) {
	reader := bufio.NewReader(conn)
	var handshake protocol.DataHandshake
	if err := json.NewDecoder(reader).Decode(&handshake); err != nil {
		return nil, protocol.DataHandshake{}, err
	}
	if handshake.Token == "" || handshake.DeviceID == "" || handshake.RequestID == "" {
		return nil, protocol.DataHandshake{}, errors.New("invalid data handshake")
	}
	return bufferedConn{Conn: conn, reader: reader}, handshake, nil
}

func (c bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (b *Broker) relay(dataConn, visitor net.Conn, onComplete CompleteFunc) error {
	defer dataConn.Close()
	defer visitor.Close()
	uploadCh := make(chan copyResult, 1)
	downloadCh := make(chan copyResult, 1)
	go copyMeasured(dataConn, visitor, uploadCh, b.meter)
	go copyMeasured(visitor, dataConn, downloadCh, b.meter)
	upload, download, err := waitCopies(dataConn, visitor, uploadCh, downloadCh)
	if onComplete != nil {
		onComplete(upload, download)
	}
	return err
}

func waitCopies(
	dataConn net.Conn,
	visitor net.Conn,
	uploadCh <-chan copyResult,
	downloadCh <-chan copyResult,
) (int64, int64, error) {
	var upload copyResult
	var download copyResult
	var firstErr error
	uploadDone := false
	select {
	case upload = <-uploadCh:
		firstErr = normalizeCopyError(upload.err)
		uploadDone = true
	case download = <-downloadCh:
		firstErr = normalizeCopyError(download.err)
	}
	_ = dataConn.Close()
	_ = visitor.Close()
	if !uploadDone {
		upload = <-uploadCh
	}
	if uploadDone {
		download = <-downloadCh
	}
	return upload.n, download.n, firstErr
}

func copyMeasured(dst net.Conn, src net.Conn, ch chan<- copyResult, meter *bandwidthMeter) {
	n, err := io.Copy(countingWriter{Writer: dst, meter: meter}, src)
	ch <- copyResult{n: n, err: err}
}

type countingWriter struct {
	io.Writer
	meter *bandwidthMeter
}

func (w countingWriter) Write(data []byte) (int, error) {
	written, err := w.Writer.Write(data)
	w.meter.Add(int64(written))
	return written, err
}

func normalizeCopyError(err error) error {
	if err == nil || err == io.EOF || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

type copyResult struct {
	n   int64
	err error
}
