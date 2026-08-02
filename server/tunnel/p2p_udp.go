package tunnel

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"

	netx "github.com/nat-link/nat-link/internal/advanced"
	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/protocol"
)

type p2pDirectSession struct {
	conn      net.PacketConn
	peer      netx.Endpoint
	sessionID string
	key       []byte
	sendSeq   uint64
	recvSeq   uint64
	closeOnce sync.Once
	release   func()
}

func (s *p2pDirectSession) close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.conn != nil {
			_ = s.conn.Close()
		}
		if s.release != nil {
			s.release()
		}
	})
}

func (s *udpVisitorSession) closeP2P() {
	s.p2pMu.Lock()
	direct := s.p2p
	s.p2p = nil
	s.p2pMu.Unlock()
	direct.close()
}

func (m *Manager) SetRendezvousAddress(address string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rdvAddress = address
}

func (m *Manager) tryP2PUDPPacket(tunnel model.Tunnel, session *udpVisitorSession, data []byte) bool {
	rdvAddress := m.rendezvousAddress()
	if rdvAddress == "" {
		return false
	}
	session.p2pMu.Lock()
	defer session.p2pMu.Unlock()
	if session.p2p == nil {
		runtime := m.udpRuntimeFor("", tunnel.ID)
		if runtime == nil || !runtime.acquireP2P() {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		direct, err := m.openP2PUDP(ctx, tunnel, session, rdvAddress)
		cancel()
		if err != nil {
			runtime.releaseP2P()
			return false
		}
		direct.release = runtime.releaseP2P
		session.p2p = direct
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	response, sent, err := directRoundTrip(ctx, session.p2p, data)
	if err != nil {
		session.p2p.close()
		session.p2p = nil
		return sent
	}
	runtime := m.udpRuntimeFor("", tunnel.ID)
	return runtime != nil && runtime.sendToVisitor(session.id, response) == nil
}

func (m *Manager) rendezvousAddress() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rdvAddress
}

func (m *Manager) openP2PUDP(
	ctx context.Context,
	tunnel model.Tunnel,
	session *udpVisitorSession,
	rdvAddress string,
) (*p2pDirectSession, error) {
	conn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	server, err := net.ResolveUDPAddr("udp", rdvAddress)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	payload, key, err := p2pPayload(rdvAddress, tunnel, session)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := m.sendP2PConnect(tunnel.ClientID, payload); err != nil {
		_ = conn.Close()
		return nil, err
	}
	result, err := netx.Rendezvous(ctx, conn, server, p2pRegister(payload))
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := punchPeer(ctx, conn, result.Peer, payload.PeerID); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &p2pDirectSession{conn: conn, peer: result.Peer, sessionID: payload.SessionID, key: key}, nil
}

func (m *Manager) sendP2PConnect(clientID string, payload protocol.P2PConnectPayload) error {
	message, err := protocol.NewMessage(protocol.TypeP2PConnect, payload.SessionID, "", payload)
	if err != nil {
		return err
	}
	return m.hub.SendControl(clientID, message)
}

func p2pPayload(
	address string,
	tunnel model.Tunnel,
	session *udpVisitorSession,
) (protocol.P2PConnectPayload, []byte, error) {
	serverID := "server-" + session.id
	agentID := "agent-" + session.id
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return protocol.P2PConnectPayload{}, nil, err
	}
	payload := protocol.P2PConnectPayload{
		RendezvousAddress: address, SessionID: uuid.NewString(),
		SessionKey: base64.RawStdEncoding.EncodeToString(key),
		PeerID:     agentID, WantsPeerID: serverID,
		LocalHost: tunnel.LocalHost, LocalPort: tunnel.LocalPort,
	}
	return payload, key, nil
}

func p2pRegister(payload protocol.P2PConnectPayload) netx.RendezvousPacket {
	return netx.RendezvousPacket{
		Type: netx.PacketRegister, SessionID: payload.SessionID,
		PeerID: payload.WantsPeerID, WantsPeerID: payload.PeerID,
	}
}

func punchPeer(ctx context.Context, conn net.PacketConn, peer netx.Endpoint, selfID string) error {
	punchCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := netx.PunchHandshake(punchCtx, conn, peer, selfID); err != nil {
		return fmt.Errorf("p2p punch: %w", err)
	}
	return nil
}

func directRoundTrip(ctx context.Context, session *p2pDirectSession, data []byte) ([]byte, bool, error) {
	addr, err := session.peer.UDPAddr()
	if err != nil {
		return nil, false, err
	}
	session.sendSeq++
	frame, err := netx.EncodeP2PFrame(session.key, netx.P2PDirectionServerToAgent, session.sendSeq, data)
	if err != nil {
		return nil, false, err
	}
	if _, err := session.conn.WriteTo(frame, addr); err != nil {
		return nil, false, err
	}
	return readDirectResponse(ctx, session, addr)
}

func readDirectResponse(
	ctx context.Context,
	session *p2pDirectSession,
	peerAddr *net.UDPAddr,
) ([]byte, bool, error) {
	buffer := make([]byte, 64*1024)
	for ctx.Err() == nil {
		_ = session.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, source, err := session.conn.ReadFrom(buffer)
		if err != nil {
			continue
		}
		if !netx.IsExpectedUDPPeer(source, peerAddr) {
			continue
		}
		if isP2PControlPacket(buffer[:n]) {
			continue
		}
		payload, sequence, err := netx.DecodeP2PFrame(
			session.key, netx.P2PDirectionAgentToServer, session.recvSeq, buffer[:n],
		)
		if err != nil {
			continue
		}
		session.recvSeq = sequence
		return payload, true, nil
	}
	return nil, true, fmt.Errorf("p2p response timeout: %w", ctx.Err())
}

func isP2PControlPacket(data []byte) bool {
	var packet netx.RendezvousPacket
	if err := json.Unmarshal(data, &packet); err != nil {
		return false
	}
	return packet.Type == netx.PacketPunch || packet.Type == netx.PacketPunchAck
}
