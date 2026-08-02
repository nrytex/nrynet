package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/nat-link/nat-link/internal/protocol"
)

type Agent struct {
	options Options
	logger  *slog.Logger
	udp     *udpRelay
}

type controlConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func New(options Options, logger *slog.Logger) (*Agent, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}
	return &Agent{options: options, logger: logger, udp: newUDPRelay(2 * time.Minute)}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	backoff := a.options.ReconnectMin
	for ctx.Err() == nil {
		err := a.runSession(ctx)
		if ctx.Err() != nil {
			return nil
		}
		a.logger.Warn("agent control session ended", "error", err, "retry_in", backoff)
		sleep(ctx, backoff)
		backoff = nextBackoff(backoff, a.options.ReconnectMax)
	}
	return nil
}

func (a *Agent) runSession(ctx context.Context) error {
	conn, err := a.dialControl(ctx)
	if err != nil {
		return err
	}
	defer conn.conn.Close()
	if err := a.sendHello(conn); err != nil {
		return err
	}
	errCh := make(chan error, 2)
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { errCh <- a.heartbeat(sessionCtx, conn) }()
	go func() { errCh <- a.readLoop(sessionCtx, conn) }()
	err = <-errCh
	cancel()
	return err
}

func (a *Agent) dialControl(ctx context.Context) (*controlConn, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  tlsConfig(a.options.Config.InsecureSkipVerify),
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+a.options.Config.Token)
	header.Set("X-NAT-Link-Device-ID", a.options.Config.DeviceID)
	conn, _, err := dialer.DialContext(ctx, a.options.Config.ServerURL, header)
	if err != nil {
		return nil, fmt.Errorf("dial control websocket: %w", err)
	}
	return &controlConn{conn: conn}, nil
}

func (a *Agent) readLoop(ctx context.Context, conn *controlConn) error {
	for ctx.Err() == nil {
		var message protocol.ControlMessage
		if err := conn.conn.ReadJSON(&message); err != nil {
			return fmt.Errorf("read control message: %w", err)
		}
		if err := a.handleControlMessage(ctx, conn, message); err != nil {
			a.logger.Warn("control message failed", "type", message.Type, "error", err)
		}
	}
	return nil
}

func (a *Agent) handleControlMessage(
	ctx context.Context,
	conn *controlConn,
	message protocol.ControlMessage,
) error {
	switch message.Type {
	case protocol.TypeTunnelSnapshot:
		return a.handleTunnelSnapshot(message)
	case protocol.TypeOpenConnection:
		go a.handleOpenConnection(ctx, message)
		return nil
	case protocol.TypeUDPPacket:
		return a.handleUDPPacket(ctx, conn, message)
	case protocol.TypeError:
		payload, err := protocol.DecodePayload[protocol.ErrorPayload](message)
		if err != nil {
			return err
		}
		return fmt.Errorf("server error: %s", payload.Message)
	default:
		a.logger.Debug("ignored control message", "type", message.Type)
		return nil
	}
}

func (a *Agent) sendHello(conn *controlConn) error {
	payload := protocol.HelloPayload{
		Name:     a.options.Config.Name,
		DeviceID: a.options.Config.DeviceID,
		OS:       runtime.GOOS,
		Version:  a.options.Version,
	}
	message, err := protocol.NewMessage(protocol.TypeHello, "", "", payload)
	if err != nil {
		return err
	}
	return conn.writeJSON(message)
}

func (c *controlConn) writeJSON(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(value)
}

func tlsConfig(skipVerify bool) *tls.Config {
	if !skipVerify {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: true}
}
