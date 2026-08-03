package agent

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/protocol"
)

type udpRelay struct {
	idle time.Duration

	mu       sync.Mutex
	sessions map[string]*udpSession
}

type udpSession struct {
	key    string
	conn   *net.UDPConn
	writer controlConn
	relay  *udpRelay
	agent  *Agent
}

func newUDPRelay(idle time.Duration) *udpRelay {
	return &udpRelay{idle: idle, sessions: make(map[string]*udpSession)}
}

func (a *Agent) handleUDPPacket(
	ctx context.Context,
	writer controlConn,
	message protocol.ControlMessage,
) error {
	payload, err := protocol.DecodePayload[protocol.UDPPacketPayload](message)
	if err != nil {
		return err
	}
	if len(payload.Payload) == 0 {
		return nil
	}
	session, err := a.ensureUDP().session(ctx, a, writer, message, payload)
	if err != nil {
		return err
	}
	n, err := session.conn.Write(payload.Payload)
	if err == nil {
		a.notifyTransfer(message.TunnelID, DirectionDownload, int64(n))
	}
	return err
}

func (a *Agent) ensureUDP() *udpRelay {
	if a.udp != nil {
		return a.udp
	}
	a.udp = newUDPRelay(2 * time.Minute)
	return a.udp
}

func (r *udpRelay) session(
	ctx context.Context,
	agent *Agent,
	writer controlConn,
	message protocol.ControlMessage,
	payload protocol.UDPPacketPayload,
) (*udpSession, error) {
	key := message.TunnelID + ":" + message.RequestID
	r.mu.Lock()
	session := r.sessions[key]
	r.mu.Unlock()
	if session != nil {
		return session, nil
	}
	return r.createSession(ctx, agent, writer, message, payload, key)
}

func (r *udpRelay) createSession(
	ctx context.Context,
	agent *Agent,
	writer controlConn,
	message protocol.ControlMessage,
	payload protocol.UDPPacketPayload,
	key string,
) (*udpSession, error) {
	address := net.JoinHostPort(payload.LocalHost, strconv.Itoa(payload.LocalPort))
	remote, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("resolve local udp service: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return nil, fmt.Errorf("dial local udp service: %w", err)
	}
	session := &udpSession{key: key, conn: conn, writer: writer, relay: r, agent: agent}
	r.mu.Lock()
	if existing := r.sessions[key]; existing != nil {
		r.mu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	r.sessions[key] = session
	r.mu.Unlock()
	go session.readLoop(context.WithoutCancel(ctx), message)
	return session, nil
}

func (s *udpSession) readLoop(ctx context.Context, source protocol.ControlMessage) {
	defer s.close()
	buffer := make([]byte, 64*1024)
	for ctx.Err() == nil {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.relay.idle))
		n, err := s.conn.Read(buffer)
		if err != nil {
			return
		}
		if err := s.send(source, buffer[:n]); err != nil {
			return
		}
	}
}

func (s *udpSession) send(source protocol.ControlMessage, data []byte) error {
	payload := protocol.UDPPacketPayload{Payload: append([]byte(nil), data...)}
	message, err := protocol.NewMessage(protocol.TypeUDPPacket, source.RequestID, source.TunnelID, payload)
	if err != nil {
		return err
	}
	if err := s.writer.writeJSON(message); err != nil {
		return err
	}
	s.agent.notifyTransfer(source.TunnelID, DirectionUpload, int64(len(data)))
	return nil
}

func (s *udpSession) close() {
	s.relay.mu.Lock()
	delete(s.relay.sessions, s.key)
	s.relay.mu.Unlock()
	_ = s.conn.Close()
}
