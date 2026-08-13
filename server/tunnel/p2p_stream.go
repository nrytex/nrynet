package tunnel

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
)

const (
	p2pStreamSetupTimeout = 2 * time.Second
	p2pRetryCooldown      = 30 * time.Second
	p2pRuntimeCooldown    = 5 * time.Second
)

func (m *Manager) SetP2PEnabled(enabled bool) {
	m.mu.Lock()
	m.p2pEnabled = enabled
	m.mu.Unlock()
}

func (m *Manager) p2pEnabledNow() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.p2pEnabled
}

func (m *Manager) ApplySetting(_ context.Context, key, value string) error {
	if key != "server.p2p_enabled" {
		return nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	m.SetP2PEnabled(enabled)
	return nil
}

func (m *Manager) tryP2PStream(tunnel model.Tunnel, visitor net.Conn) bool {
	// TCP is the explicit relay protocol. Only a tunnel configured as P2P may
	// attempt the UDP-hole-punched QUIC stream path.
	if tunnel.Protocol != "p2p" || !m.p2pEnabledNow() {
		return false
	}
	if !m.p2pRetryAllowed(tunnel.ID) {
		return false
	}
	rendezvous := m.rendezvousAddress()
	if rendezvous == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), p2pStreamSetupTimeout)
	stream, err := m.openP2PStream(ctx, tunnel, rendezvous)
	cancel()
	if err != nil {
		m.deferP2PRetry(tunnel.ID)
		m.recordEvent(context.Background(), "info", "p2p.tcp.fallback",
			"P2P stream setup failed; using relay", map[string]any{
				"tunnel_id": tunnel.ID, "error": err.Error(),
			})
		return false
	}
	m.clearP2PRetry(tunnel.ID)
	m.recordEvent(context.Background(), "info", "p2p.tcp.direct",
		"P2P stream path established", map[string]any{"tunnel_id": tunnel.ID})
	m.notifyTunnelPath(tunnel, protocol.TunnelPathP2P)
	m.active.Add(1)
	defer m.active.Add(-1)
	err = m.broker.RelayStream(stream, visitor, func(upload, download int64) {
		m.recordTraffic(tunnel.ID, upload, download)
	})
	if err != nil {
		// A stream can fail after ICE/QUIC setup succeeds. Keep the next
		// visitor on Relay for a short window instead of repeatedly sending
		// traffic into the same broken direct path.
		m.deferP2PRetryFor(tunnel.ID, p2pRuntimeCooldown)
		m.recordConnectionFailure(tunnel.ID, "p2p", err)
	}
	return true
}

func (m *Manager) p2pRetryAllowed(tunnelID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.p2pRetryAt == nil {
		m.p2pRetryAt = make(map[string]time.Time)
	}
	until, ok := m.p2pRetryAt[tunnelID]
	if !ok {
		return true
	}
	if time.Now().After(until) {
		delete(m.p2pRetryAt, tunnelID)
		return true
	}
	return false
}

func (m *Manager) deferP2PRetry(tunnelID string) {
	m.deferP2PRetryFor(tunnelID, p2pRetryCooldown)
}

func (m *Manager) deferP2PRetryFor(tunnelID string, cooldown time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.p2pRetryAt == nil {
		m.p2pRetryAt = make(map[string]time.Time)
	}
	m.p2pRetryAt[tunnelID] = time.Now().Add(cooldown)
}

func (m *Manager) clearP2PRetry(tunnelID string) {
	m.mu.Lock()
	delete(m.p2pRetryAt, tunnelID)
	m.mu.Unlock()
}

func (m *Manager) openP2PStream(
	ctx context.Context,
	tunnel model.Tunnel,
	rendezvous string,
) (*p2pQUICStream, error) {
	conn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	var session *netx.QUICSession
	keep := false
	defer func() {
		if keep {
			return
		}
		if session != nil {
			_ = session.Close()
		}
		_ = conn.Close()
	}()
	server, err := net.ResolveUDPAddr("udp", rendezvous)
	if err != nil {
		return nil, err
	}
	requestID := uuid.NewString()
	payload, key, err := newP2PStreamPayload(rendezvous, tunnel, requestID)
	if err != nil {
		return nil, err
	}
	if err := m.sendP2PConnect(tunnel.ClientID, payload); err != nil {
		return nil, err
	}
	result, err := netx.Rendezvous(ctx, conn, server, p2pRegister(payload))
	if err != nil {
		return nil, err
	}
	if err := punchPeer(ctx, conn, result.Peer, payload.PeerID); err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Time{})
	proof, err := netx.P2PProof(key, payload.SessionID, payload.RequestID, netx.P2PStreamRoleServer)
	if err != nil {
		return nil, err
	}
	peer, err := result.Peer.UDPAddr()
	if err != nil {
		return nil, err
	}
	tlsConfig := netx.ClientTLSConfig("nrynet-p2p", true)
	session, err = netx.DialQUICPacketConn(ctx, conn, peer, tlsConfig, netx.AuthRequest{
		Token: proof, DeviceID: payload.SessionID, Role: netx.P2PStreamRoleServer,
	})
	if err != nil {
		return nil, err
	}
	stream, err := session.OpenStreamFrame(ctx, netx.Frame{
		Kind: netx.FrameData, RequestID: payload.RequestID, TunnelID: tunnel.ID,
	})
	if err != nil {
		return nil, err
	}
	if err := verifyP2PAgentAuth(stream, key, payload); err != nil {
		return nil, err
	}
	keep = true
	return &p2pQUICStream{session: session, stream: stream, packet: conn}, nil
}

func newP2PStreamPayload(
	rendezvous string,
	tunnel model.Tunnel,
	requestID string,
) (protocol.P2PConnectPayload, []byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return protocol.P2PConnectPayload{}, nil, err
	}
	serverID := "server-" + requestID
	agentID := "agent-" + requestID
	return protocol.P2PConnectPayload{
		RendezvousAddress: rendezvous,
		SessionID:         uuid.NewString(),
		SessionKey:        base64.RawStdEncoding.EncodeToString(key),
		PeerID:            serverID,
		WantsPeerID:       agentID,
		LocalHost:         tunnel.LocalHost,
		LocalPort:         tunnel.LocalPort,
		Mode:              protocol.P2PModeStream,
		RequestID:         requestID,
		TunnelID:          tunnel.ID,
	}, key, nil
}

func verifyP2PAgentAuth(
	stream *netx.QUICStream,
	key []byte,
	payload protocol.P2PConnectPayload,
) error {
	frame, err := netx.ReadFrame(stream)
	if err != nil {
		return err
	}
	if frame.Kind != netx.FrameAuth || !netx.VerifyP2PProof(
		key, payload.SessionID, payload.RequestID, netx.P2PStreamRoleAgent, string(frame.Payload),
	) {
		return errors.New("invalid p2p agent proof")
	}
	return nil
}

type p2pQUICStream struct {
	stream  *netx.QUICStream
	session *netx.QUICSession
	packet  net.PacketConn
	once    sync.Once
}

func (s *p2pQUICStream) Read(data []byte) (int, error)  { return s.stream.Read(data) }
func (s *p2pQUICStream) Write(data []byte) (int, error) { return s.stream.Write(data) }

func (s *p2pQUICStream) Close() error {
	var err error
	s.once.Do(func() {
		err = errors.Join(s.stream.Close(), s.session.Close(), s.packet.Close())
	})
	return err
}
