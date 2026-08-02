package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	netx "github.com/nat-link/nat-link/internal/advanced"
	"github.com/nat-link/nat-link/internal/protocol"
)

type Agent struct {
	options Options
	logger  *slog.Logger
	udp     *udpRelay
}

type controlConn interface {
	readJSON(value any) error
	writeJSON(value any) error
	close() error
	openData(context.Context, string) (dataConn, error)
}

type dataConn interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
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
	defer conn.close()
	if err := a.sendHello(conn); err != nil {
		return err
	}
	a.notifySessionStarted()
	errCh := make(chan error, 2)
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { errCh <- a.heartbeat(sessionCtx, conn) }()
	go func() { errCh <- a.readLoop(sessionCtx, conn) }()
	err = <-errCh
	cancel()
	a.notifySessionEnded(err)
	return err
}

func (a *Agent) dialControl(ctx context.Context) (controlConn, error) {
	if a.options.Config.Transport == "quic" {
		return a.dialQUICControl(ctx)
	}
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
	return &websocketControl{conn: conn, agent: a}, nil
}

func (a *Agent) dialQUICControl(ctx context.Context) (controlConn, error) {
	tlsConfig := netx.ClientTLSConfig(quicServerName(a.options.Config.QUICAddress), a.options.Config.InsecureSkipVerify)
	session, err := netx.DialQUIC(ctx, a.options.Config.QUICAddress, tlsConfig, netx.AuthRequest{
		Token: a.options.Config.Token, DeviceID: a.options.Config.DeviceID, Role: "agent",
	})
	if err != nil {
		return nil, err
	}
	stream, err := session.OpenStream(ctx, netx.FrameControl)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	return &quicControl{session: session, stream: stream}, nil
}

func (a *Agent) readLoop(ctx context.Context, conn controlConn) error {
	for ctx.Err() == nil {
		var message protocol.ControlMessage
		if err := conn.readJSON(&message); err != nil {
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
	conn controlConn,
	message protocol.ControlMessage,
) error {
	switch message.Type {
	case protocol.TypeTunnelSnapshot:
		return a.handleTunnelSnapshot(message)
	case protocol.TypeOpenConnection:
		go a.handleOpenConnection(ctx, conn, message)
		return nil
	case protocol.TypeUDPPacket:
		return a.handleUDPPacket(ctx, conn, message)
	case protocol.TypeP2PConnect:
		go a.handleP2PConnect(ctx, message)
		return nil
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

func (a *Agent) sendHello(conn controlConn) error {
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

type websocketControl struct {
	conn  *websocket.Conn
	agent *Agent
	mu    sync.Mutex
}

func (c *websocketControl) readJSON(value any) error {
	return c.conn.ReadJSON(value)
}

func (c *websocketControl) writeJSON(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(value)
}

func (c *websocketControl) close() error {
	return c.conn.Close()
}

func (c *websocketControl) openData(ctx context.Context, _ string) (dataConn, error) {
	return c.agent.dialLegacyData(ctx)
}

type quicControl struct {
	session *netx.QUICSession
	stream  *netx.QUICStream
	mu      sync.Mutex
}

func (c *quicControl) readJSON(value any) error {
	frame, err := netx.ReadFrame(c.stream)
	if err != nil {
		return err
	}
	return json.Unmarshal(frame.Payload, value)
}

func (c *quicControl) writeJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return netx.WriteFrame(c.stream, netx.Frame{Kind: netx.FrameControl, Payload: data})
}

func (c *quicControl) close() error {
	_ = c.stream.Close()
	return c.session.Close()
}

func (c *quicControl) openData(ctx context.Context, requestID string) (dataConn, error) {
	return c.session.OpenStreamFrame(ctx, netx.Frame{Kind: netx.FrameData, RequestID: requestID})
}

func tlsConfig(skipVerify bool) *tls.Config {
	if !skipVerify {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: true}
}

func quicServerName(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return "localhost"
	}
	return host
}
