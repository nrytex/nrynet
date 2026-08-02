package advanced

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

type Authenticator func(context.Context, AuthRequest, net.Addr) error

type QUICServer struct {
	listener *quic.Listener
	auth     Authenticator
}

type QUICSession struct {
	conn   *quic.Conn
	PeerID string
	Auth   AuthRequest
}

type QUICStream struct {
	*quic.Stream
	Initial Frame
	Kind    string
}

func ListenQUIC(addr string, tlsConfig *tls.Config, auth Authenticator) (*QUICServer, error) {
	if auth == nil {
		return nil, errors.New("authenticator is required")
	}
	listener, err := quic.ListenAddr(addr, tlsConfig, quicConfig())
	if err != nil {
		return nil, fmt.Errorf("listen quic: %w", err)
	}
	return &QUICServer{listener: listener, auth: auth}, nil
}

func DialQUIC(
	ctx context.Context,
	addr string,
	tlsConfig *tls.Config,
	request AuthRequest,
) (*QUICSession, error) {
	conn, err := quic.DialAddr(ctx, addr, tlsConfig, quicConfig())
	if err != nil {
		return nil, fmt.Errorf("dial quic: %w", err)
	}
	if err := sendAuth(ctx, conn, request); err != nil {
		_ = conn.CloseWithError(1, err.Error())
		return nil, err
	}
	return &QUICSession{conn: conn, PeerID: request.DeviceID, Auth: request}, nil
}

func (s *QUICServer) Addr() net.Addr {
	return s.listener.Addr()
}

func (s *QUICServer) Close() error {
	return s.listener.Close()
}

func (s *QUICServer) Accept(ctx context.Context) (*QUICSession, error) {
	conn, err := s.listener.Accept(ctx)
	if err != nil {
		return nil, fmt.Errorf("accept quic: %w", err)
	}
	auth, err := readAuth(ctx, conn)
	if err != nil {
		_ = conn.CloseWithError(1, err.Error())
		return nil, err
	}
	if err := s.auth(ctx, auth, conn.RemoteAddr()); err != nil {
		_ = conn.CloseWithError(2, "unauthorized")
		return nil, fmt.Errorf("authenticate quic: %w", err)
	}
	return &QUICSession{conn: conn, PeerID: auth.DeviceID, Auth: auth}, nil
}

func (s *QUICSession) OpenStream(ctx context.Context, kind string) (*QUICStream, error) {
	return s.OpenStreamFrame(ctx, Frame{Kind: kind})
}

func (s *QUICSession) OpenStreamFrame(ctx context.Context, frame Frame) (*QUICStream, error) {
	stream, err := s.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open quic stream: %w", err)
	}
	if err := WriteFrame(stream, frame); err != nil {
		_ = stream.Close()
		return nil, err
	}
	return &QUICStream{Stream: stream, Initial: frame, Kind: frame.Kind}, nil
}

func (s *QUICSession) AcceptStream(ctx context.Context) (*QUICStream, error) {
	stream, err := s.conn.AcceptStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("accept quic stream: %w", err)
	}
	frame, err := ReadFrame(stream)
	if err != nil {
		_ = stream.Close()
		return nil, err
	}
	return &QUICStream{Stream: stream, Initial: frame, Kind: frame.Kind}, nil
}

func (s *QUICSession) Close() error {
	return s.conn.CloseWithError(0, "")
}

func (s *QUICSession) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

func sendAuth(ctx context.Context, conn *quic.Conn, request AuthRequest) error {
	payload, err := EncodeAuth(request)
	if err != nil {
		return err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("open auth stream: %w", err)
	}
	defer stream.Close()
	return WriteFrame(stream, Frame{Kind: FrameAuth, Payload: payload})
}

func readAuth(ctx context.Context, conn *quic.Conn) (AuthRequest, error) {
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return AuthRequest{}, fmt.Errorf("accept auth stream: %w", err)
	}
	defer stream.Close()
	frame, err := ReadFrame(stream)
	if err != nil {
		return AuthRequest{}, err
	}
	return DecodeAuth(frame)
}

func quicConfig() *quic.Config {
	return &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}
}
