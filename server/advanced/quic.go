package advanced

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	netx "github.com/nat-link/nat-link/internal/advanced"
	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/protocol"
	clienthub "github.com/nat-link/nat-link/server/client"
	"github.com/nat-link/nat-link/server/relay"
)

type QUICControlServer struct {
	server *netx.QUICServer
	auth   *auth.Service
	hub    *clienthub.Hub
	broker *relay.Broker
}

func ListenQUIC(
	addr string,
	tlsConfig *tls.Config,
	authService *auth.Service,
	hub *clienthub.Hub,
	broker *relay.Broker,
) (*QUICControlServer, error) {
	server, err := netx.ListenQUIC(addr, tlsConfig, authFunc(authService))
	if err != nil {
		return nil, err
	}
	return &QUICControlServer{server: server, auth: authService, hub: hub, broker: broker}, nil
}

func (s *QUICControlServer) Addr() net.Addr {
	return s.server.Addr()
}

func (s *QUICControlServer) Close() error {
	return s.server.Close()
}

func (s *QUICControlServer) Serve(ctx context.Context) error {
	for ctx.Err() == nil {
		session, err := s.server.Accept(ctx)
		if err != nil {
			return err
		}
		go s.handleSession(ctx, session)
	}
	return nil
}

func (s *QUICControlServer) handleSession(ctx context.Context, session *netx.QUICSession) {
	defer session.Close()
	token, err := s.auth.AuthenticateAgent(ctx, session.Auth.Token)
	if err != nil {
		return
	}
	for ctx.Err() == nil {
		stream, err := session.AcceptStream(ctx)
		if err != nil {
			return
		}
		go s.handleStream(ctx, session, stream, token.ID)
	}
}

func (s *QUICControlServer) handleStream(
	ctx context.Context,
	session *netx.QUICSession,
	stream *netx.QUICStream,
	tokenID string,
) {
	switch stream.Kind {
	case netx.FrameControl:
		ip := hostOnly(session.RemoteAddr())
		s.hub.ServeTransport(ctx, &quicControl{stream: stream, session: session}, tokenID, ip)
	case netx.FrameData:
		handshake := protocol.DataHandshake{
			DeviceID:  session.Auth.DeviceID,
			RequestID: stream.Initial.RequestID,
		}
		go s.broker.HandleAuthenticatedStream(stream, handshake, tokenID)
	default:
		_ = stream.Close()
	}
}

func authFunc(authService *auth.Service) netx.Authenticator {
	return func(ctx context.Context, request netx.AuthRequest, _ net.Addr) error {
		_, err := authService.AuthenticateAgent(ctx, request.Token)
		return err
	}
}

func hostOnly(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

type quicControl struct {
	stream  *netx.QUICStream
	session *netx.QUICSession
	mu      sync.Mutex
}

func (c *quicControl) ReadJSON(value any) error {
	frame, err := netx.ReadFrame(c.stream)
	if err != nil {
		return err
	}
	return json.Unmarshal(frame.Payload, value)
}

func (c *quicControl) WriteJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return netx.WriteFrame(c.stream, netx.Frame{Kind: netx.FrameControl, Payload: data})
}

func (c *quicControl) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session.Close()
}

func (c *quicControl) SetReadDeadline(deadline time.Time) error {
	if setter, ok := any(c.stream.Stream).(interface{ SetReadDeadline(time.Time) error }); ok {
		return setter.SetReadDeadline(deadline)
	}
	return fmt.Errorf("quic stream deadline unsupported")
}
