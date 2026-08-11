package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	clientadvanced "github.com/nrytex/nrynet/client/advanced"
	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/protocol"
)

func (a *Agent) handleP2PConnect(ctx context.Context, message protocol.ControlMessage) {
	payload, err := protocol.DecodePayload[protocol.P2PConnectPayload](message)
	if err == nil && payload.Mode == protocol.P2PModeStream {
		err = a.runP2PStream(ctx, payload)
	} else if err == nil {
		err = a.runP2PUDP(ctx, payload)
	}
	if err != nil {
		a.logger.Debug("p2p path unavailable", "mode", payload.Mode, "error", err)
	}
}

func (a *Agent) runP2PUDP(ctx context.Context, payload protocol.P2PConnectPayload) error {
	var err error
	conn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return err
	}
	defer conn.Close()
	server, err := net.ResolveUDPAddr("udp", payload.RendezvousAddress)
	if err != nil {
		return err
	}
	result, err := registerP2PPeer(ctx, conn, server, payload)
	if err != nil {
		return err
	}
	direct := clientadvanced.PeerConnector{Timeout: time.Second}.PunchOrRelay(
		ctx, conn, result.Peer, payload.WantsPeerID,
	)
	if direct.UseRelay {
		return fmt.Errorf("p2p fallback: %s", direct.RelayReason)
	}
	return a.proxyP2PDatagrams(ctx, conn, payload, result.Peer)
}

func registerP2PPeer(
	ctx context.Context,
	conn net.PacketConn,
	server net.Addr,
	payload protocol.P2PConnectPayload,
) (netx.RendezvousResult, error) {
	return netx.Rendezvous(ctx, conn, server, netx.RendezvousPacket{
		Type: netx.PacketRegister, SessionID: payload.SessionID,
		PeerID: payload.PeerID, WantsPeerID: payload.WantsPeerID,
	})
}

func (a *Agent) proxyP2PDatagrams(
	ctx context.Context,
	conn net.PacketConn,
	payload protocol.P2PConnectPayload,
	peer netx.Endpoint,
) error {
	key, err := decodeP2PSessionKey(payload.SessionKey)
	if err != nil {
		return err
	}
	local, err := net.Dial("udp", net.JoinHostPort(payload.LocalHost, strconv.Itoa(payload.LocalPort)))
	if err != nil {
		return err
	}
	defer local.Close()
	stop := make(chan struct{})
	a.goWorker("p2p udp cleanup", func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
			_ = local.Close()
		case <-stop:
		}
	})
	defer close(stop)
	peerAddr, err := peer.UDPAddr()
	if err != nil {
		return err
	}
	proxy := p2pDatagramProxy{conn: conn, local: local, peer: peerAddr, key: key}
	return proxy.run(ctx)
}

func decodeP2PSessionKey(encoded string) ([]byte, error) {
	key, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid p2p session key")
	}
	return key, nil
}

type p2pDatagramProxy struct {
	conn           net.PacketConn
	local          net.Conn
	peer           *net.UDPAddr
	key            []byte
	received, sent uint64
}

func (p *p2pDatagramProxy) run(ctx context.Context) error {
	buffer := make([]byte, 64*1024)
	for ctx.Err() == nil {
		_ = p.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		n, source, err := p.conn.ReadFrom(buffer)
		if err != nil {
			return err
		}
		if !netx.IsExpectedUDPPeer(source, p.peer) {
			continue
		}
		if isP2PControlPacket(buffer[:n]) {
			continue
		}
		if err := p.handle(buffer[:n]); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (p *p2pDatagramProxy) handle(frame []byte) error {
	request, sequence, err := netx.DecodeP2PFrame(
		p.key, netx.P2PDirectionServerToAgent, p.received, frame,
	)
	if err != nil {
		return nil
	}
	p.received = sequence
	response, err := roundTripLocalUDP(p.local, request)
	if err != nil {
		return err
	}
	p.sent++
	frame, err = netx.EncodeP2PFrame(p.key, netx.P2PDirectionAgentToServer, p.sent, response)
	if err != nil {
		return err
	}
	_, err = p.conn.WriteTo(frame, p.peer)
	return err
}

func isP2PControlPacket(data []byte) bool {
	var packet netx.RendezvousPacket
	if err := json.Unmarshal(data, &packet); err != nil {
		return false
	}
	return packet.Type == netx.PacketPunch || packet.Type == netx.PacketPunchAck
}

func roundTripLocalUDP(local net.Conn, data []byte) ([]byte, error) {
	if _, err := local.Write(data); err != nil {
		return nil, err
	}
	buffer := make([]byte, 64*1024)
	_ = local.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := local.Read(buffer)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), buffer[:n]...), nil
}
