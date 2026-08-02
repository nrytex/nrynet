package advanced

import (
	"context"
	"errors"
	"net"
	"time"

	netx "github.com/nat-link/nat-link/internal/advanced"
)

type DirectConnectResult struct {
	Observed    netx.Endpoint
	Peer        netx.Endpoint
	UseRelay    bool
	RelayReason string
}

func SendDirectPayload(
	ctx context.Context,
	conn net.PacketConn,
	peer netx.Endpoint,
	payload []byte,
) error {
	addr, err := peer.UDPAddr()
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(payload, addr)
	if err != nil {
		return err
	}
	return waitReadable(ctx, conn)
}

func waitReadable(ctx context.Context, conn net.PacketConn) error {
	buffer := make([]byte, 2048)
	for ctx.Err() == nil {
		_ = conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		if _, _, err := conn.ReadFrom(buffer); err == nil {
			return nil
		} else if !isNetTimeout(err) {
			return err
		}
	}
	return ctx.Err()
}

func isNetTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

type PeerConnector struct {
	Timeout time.Duration
}

func (c PeerConnector) Discover(
	ctx context.Context,
	conn net.PacketConn,
	server net.Addr,
	packet netx.RendezvousPacket,
) (DirectConnectResult, error) {
	result, err := netx.Rendezvous(ctx, conn, server, packet)
	if err != nil {
		return DirectConnectResult{}, err
	}
	return DirectConnectResult{
		Observed: result.Observed,
		Peer:     result.Peer,
		UseRelay: result.Relay,
	}, nil
}

func (c PeerConnector) PunchOrRelay(
	ctx context.Context,
	conn net.PacketConn,
	peer netx.Endpoint,
	selfID string,
) DirectConnectResult {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	punchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := netx.PunchHandshake(punchCtx, conn, peer, selfID); err == nil {
		return DirectConnectResult{Peer: peer}
	}
	return DirectConnectResult{
		Peer:        peer,
		UseRelay:    true,
		RelayReason: "udp hole punching timed out",
	}
}
