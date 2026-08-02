package advanced

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

type RendezvousResult struct {
	Observed Endpoint
	Peer     Endpoint
	PeerID   string
	Relay    bool
}

func Rendezvous(
	ctx context.Context,
	conn net.PacketConn,
	server net.Addr,
	packet RendezvousPacket,
) (RendezvousResult, error) {
	if err := validateRegister(packet); err != nil {
		return RendezvousResult{}, err
	}
	if err := writePacket(conn, server, packet); err != nil {
		return RendezvousResult{}, err
	}
	return waitRendezvous(ctx, conn)
}

func Punch(
	ctx context.Context,
	conn net.PacketConn,
	peer Endpoint,
	selfID string,
) error {
	addr, err := peer.UDPAddr()
	if err != nil {
		return err
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := writePacket(conn, addr, punchPacket(selfID)); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return ctx.Err()
}

// PunchHandshake completes only after both peers have received a punch.
// This keeps the first application datagram out of the peer's punch reader.
func PunchHandshake(
	ctx context.Context,
	conn net.PacketConn,
	peer Endpoint,
	selfID string,
) error {
	addr, err := peer.UDPAddr()
	if err != nil {
		return err
	}
	buffer := make([]byte, 1024)
	for ctx.Err() == nil {
		if err := writePacket(conn, addr, punchPacket(selfID)); err != nil {
			return err
		}
		acknowledged, err := awaitPunchAcknowledgement(ctx, conn, addr, buffer)
		if err != nil {
			return err
		}
		if acknowledged {
			return nil
		}
	}
	return ctx.Err()
}

func awaitPunchAcknowledgement(
	ctx context.Context,
	conn net.PacketConn,
	peer *net.UDPAddr,
	buffer []byte,
) (bool, error) {
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	n, source, err := conn.ReadFrom(buffer)
	if err != nil {
		if isTimeout(err) {
			return false, nil
		}
		return false, err
	}
	if !IsExpectedUDPPeer(source, peer) {
		return false, nil
	}
	packet, err := decodePacket(buffer[:n])
	if err != nil {
		return false, nil
	}
	if packet.Type == PacketPunch {
		_ = writePacket(conn, source, RendezvousPacket{Type: PacketPunchAck})
		return false, nil
	}
	return packet.Type == PacketPunchAck, nil
}

func AwaitPunch(ctx context.Context, conn net.PacketConn) (Endpoint, error) {
	buffer := make([]byte, 1024)
	for ctx.Err() == nil {
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, addr, err := conn.ReadFrom(buffer)
		if err != nil {
			if isTimeout(err) {
				continue
			}
			return Endpoint{}, err
		}
		packet, err := decodePacket(buffer[:n])
		if err != nil || packet.Type != PacketPunch {
			continue
		}
		_ = writePacket(conn, addr, RendezvousPacket{Type: PacketPunchAck})
		return endpointFromAddr(addr), nil
	}
	return Endpoint{}, ctx.Err()
}

func waitRendezvous(ctx context.Context, conn net.PacketConn) (RendezvousResult, error) {
	var result RendezvousResult
	buffer := make([]byte, 2048)
	for ctx.Err() == nil {
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, _, err := conn.ReadFrom(buffer)
		if err != nil {
			if isTimeout(err) {
				continue
			}
			return result, err
		}
		if applyRendezvousPacket(&result, buffer[:n]) {
			return result, nil
		}
	}
	return result, ctx.Err()
}

func applyRendezvousPacket(result *RendezvousResult, data []byte) bool {
	packet, err := decodePacket(data)
	if err != nil {
		return false
	}
	switch packet.Type {
	case PacketObserved:
		result.Observed = packet.Endpoint
	case PacketPeer:
		result.Peer = packet.Endpoint
		result.PeerID = packet.PeerID
		result.Relay = packet.Relay
	}
	return result.Observed.Port != 0 && result.Peer.Port != 0
}

func validateRegister(packet RendezvousPacket) error {
	if packet.Type != PacketRegister {
		return errors.New("rendezvous packet must be register")
	}
	if packet.SessionID == "" || packet.PeerID == "" || packet.WantsPeerID == "" {
		return errors.New("session_id, peer_id, and wants_peer_id are required")
	}
	return nil
}

func writePacket(conn net.PacketConn, addr net.Addr, packet RendezvousPacket) error {
	data, err := json.Marshal(packet)
	if err != nil {
		return fmt.Errorf("encode rendezvous packet: %w", err)
	}
	_, err = conn.WriteTo(data, addr)
	return err
}

func punchPacket(peerID string) RendezvousPacket {
	return RendezvousPacket{Type: PacketPunch, PeerID: peerID}
}

func decodePacket(data []byte) (RendezvousPacket, error) {
	var packet RendezvousPacket
	err := json.Unmarshal(data, &packet)
	return packet, err
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
