package advanced

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	defaultRendezvousTTL      = 2 * time.Minute
	defaultRendezvousMaxPeers = 10000
)

type RendezvousServer struct {
	conn  net.PacketConn
	mu    sync.Mutex
	peers map[string]registeredPeer
	ttl   time.Duration
	max   int
}

type registeredPeer struct {
	packet RendezvousPacket
	addr   net.Addr
	seen   time.Time
}

func NewRendezvousServer(conn net.PacketConn) *RendezvousServer {
	return &RendezvousServer{
		conn: conn, peers: make(map[string]registeredPeer),
		ttl: defaultRendezvousTTL, max: defaultRendezvousMaxPeers,
	}
}

func ListenRendezvous(addr string) (*RendezvousServer, error) {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen rendezvous: %w", err)
	}
	return NewRendezvousServer(conn), nil
}

func (s *RendezvousServer) Addr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *RendezvousServer) Close() error {
	return s.conn.Close()
}

func (s *RendezvousServer) Serve(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.readLoop() }()
	select {
	case <-ctx.Done():
		_ = s.conn.Close()
		return nil
	case err := <-errCh:
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
}

func (s *RendezvousServer) readLoop() error {
	buffer := make([]byte, 2048)
	for {
		n, addr, err := s.conn.ReadFrom(buffer)
		if err != nil {
			return err
		}
		var packet RendezvousPacket
		if err := json.Unmarshal(buffer[:n], &packet); err != nil {
			continue
		}
		s.handlePacket(packet, addr)
	}
}

func (s *RendezvousServer) handlePacket(packet RendezvousPacket, addr net.Addr) {
	switch packet.Type {
	case PacketRegister:
		s.register(packet, addr)
	case PacketPunch:
		_ = s.write(addr, RendezvousPacket{Type: PacketPunchAck, PeerID: packet.PeerID})
	}
}

func (s *RendezvousServer) register(packet RendezvousPacket, addr net.Addr) {
	if packet.SessionID == "" || packet.PeerID == "" || packet.WantsPeerID == "" {
		return
	}
	packet.Endpoint = endpointFromAddr(addr)
	_ = s.write(addr, observedPacket(packet))
	now := time.Now()
	s.mu.Lock()
	s.pruneLocked(now)
	key := peerKey(packet.SessionID, packet.PeerID)
	s.peers[key] = registeredPeer{packet: packet, addr: addr, seen: now}
	other := s.findPeerLocked(packet)
	if other.addr != nil {
		delete(s.peers, key)
		delete(s.peers, peerKey(packet.SessionID, packet.WantsPeerID))
	}
	s.mu.Unlock()
	if other.addr != nil {
		s.exchange(packet, addr, other)
	}
}

func (s *RendezvousServer) pruneLocked(now time.Time) {
	for key, peer := range s.peers {
		if now.Sub(peer.seen) > s.ttl {
			delete(s.peers, key)
		}
	}
	for len(s.peers) >= s.max {
		var oldestKey string
		var oldest time.Time
		for key, peer := range s.peers {
			if oldestKey == "" || peer.seen.Before(oldest) {
				oldestKey, oldest = key, peer.seen
			}
		}
		delete(s.peers, oldestKey)
	}
}

func (s *RendezvousServer) findPeerLocked(packet RendezvousPacket) registeredPeer {
	wanted := peerKey(packet.SessionID, packet.WantsPeerID)
	other := s.peers[wanted]
	if other.packet.WantsPeerID != packet.PeerID {
		return registeredPeer{}
	}
	return other
}

func (s *RendezvousServer) exchange(
	packet RendezvousPacket,
	addr net.Addr,
	other registeredPeer,
) {
	_ = s.write(addr, peerPacket(other.packet))
	_ = s.write(other.addr, peerPacket(packet))
}

func (s *RendezvousServer) write(addr net.Addr, packet RendezvousPacket) error {
	data, err := json.Marshal(packet)
	if err != nil {
		return err
	}
	_, err = s.conn.WriteTo(data, addr)
	return err
}

func observedPacket(packet RendezvousPacket) RendezvousPacket {
	return RendezvousPacket{Type: PacketObserved, Endpoint: packet.Endpoint}
}

func peerPacket(packet RendezvousPacket) RendezvousPacket {
	return RendezvousPacket{
		Type:     PacketPeer,
		PeerID:   packet.PeerID,
		Endpoint: packet.Endpoint,
		Relay:    packet.Relay,
	}
}

func peerKey(sessionID, peerID string) string {
	return sessionID + "\x00" + peerID
}
