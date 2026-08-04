package app

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
)

const tlsSniffTimeout = 2 * time.Second

type dynamicTLSListener struct {
	base      net.Listener
	tlsConfig *tls.Config
	store     *netx.DynamicTLSStore
	ready     chan net.Conn
	errCh     chan error
	closed    chan struct{}
	once      sync.Once
}

func newDynamicTLSListener(base net.Listener, store *netx.DynamicTLSStore) net.Listener {
	listener := &dynamicTLSListener{
		base:      base,
		tlsConfig: netx.ServerDynamicTLSConfig(store),
		store:     store,
		ready:     make(chan net.Conn),
		errCh:     make(chan error, 1),
		closed:    make(chan struct{}),
	}
	go listener.acceptRaw()
	return listener
}

func (l *dynamicTLSListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.ready:
		if conn == nil {
			return nil, net.ErrClosed
		}
		return conn, nil
	case err := <-l.errCh:
		return nil, err
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *dynamicTLSListener) Close() error {
	l.once.Do(func() {
		close(l.closed)
		_ = l.base.Close()
	})
	return nil
}

func (l *dynamicTLSListener) Addr() net.Addr {
	return l.base.Addr()
}

func (l *dynamicTLSListener) acceptRaw() {
	for {
		conn, err := l.base.Accept()
		if err != nil {
			l.sendAcceptError(err)
			return
		}
		go l.classify(conn)
	}
}

func (l *dynamicTLSListener) classify(conn net.Conn) {
	classified, ok := l.classifyConn(conn)
	if !ok {
		return
	}
	select {
	case l.ready <- classified:
	case <-l.closed:
		_ = classified.Close()
	}
}

func (l *dynamicTLSListener) classifyConn(conn net.Conn) (net.Conn, bool) {
	first := make([]byte, 1)
	_ = conn.SetReadDeadline(time.Now().Add(tlsSniffTimeout))
	_, err := io.ReadFull(conn, first)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		_ = conn.Close()
		return nil, false
	}
	prefixed := newPrefixConn(conn, first)
	if first[0] != 0x16 {
		return prefixed, true
	}
	if !l.store.Enabled() {
		_ = prefixed.Close()
		return nil, false
	}
	return tls.Server(prefixed, l.tlsConfig), true
}

func (l *dynamicTLSListener) sendAcceptError(err error) {
	if errors.Is(err, net.ErrClosed) {
		l.Close()
		return
	}
	select {
	case l.errCh <- err:
	case <-l.closed:
	}
}

type prefixConn struct {
	net.Conn
	reader io.Reader
}

func newPrefixConn(conn net.Conn, prefix []byte) net.Conn {
	copied := append([]byte(nil), prefix...)
	return &prefixConn{Conn: conn, reader: io.MultiReader(bytes.NewReader(copied), conn)}
}

func (c *prefixConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}
