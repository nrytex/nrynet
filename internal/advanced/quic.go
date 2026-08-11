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
	conn        *quic.Conn
	packetConn  net.PacketConn
	closePacket bool
	PeerID      string
	Auth        AuthRequest
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
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen quic packet connection: %w", err)
	}
	server, err := ListenQUICPacketConn(conn, tlsConfig, auth)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return server, nil
}

func ListenQUICPacketConn(conn net.PacketConn, tlsConfig *tls.Config, auth Authenticator) (*QUICServer, error) {
	if conn == nil {
		return nil, errors.New("quic packet connection is required")
	}
	if auth == nil {
		return nil, errors.New("authenticator is required")
	}
	listener, err := quic.Listen(conn, tlsConfig, quicConfig())
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
	packetConn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("listen quic packet connection: %w", err)
	}
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		_ = packetConn.Close()
		return nil, fmt.Errorf("resolve quic address: %w", err)
	}
	session, err := DialQUICPacketConn(ctx, packetConn, udpAddr, tlsConfig, request)
	if err != nil {
		_ = packetConn.Close()
		return nil, err
	}
	session.packetConn = packetConn
	session.closePacket = true
	return session, nil
}

func DialQUICPacketConn(
	ctx context.Context,
	packetConn net.PacketConn,
	addr net.Addr,
	tlsConfig *tls.Config,
	request AuthRequest,
) (*QUICSession, error) {
	if packetConn == nil || addr == nil {
		return nil, errors.New("quic packet connection and address are required")
	}
	conn, err := quic.Dial(ctx, packetConn, addr, tlsConfig, quicConfig())
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
	err := s.conn.CloseWithError(0, "")
	if s.closePacket && s.packetConn != nil {
		err = errors.Join(err, s.packetConn.Close())
	}
	return err
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
