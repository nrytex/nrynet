package agent

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/protocol"
)

const p2pStreamSetupTimeout = 2 * time.Second

func (a *Agent) runP2PStream(ctx context.Context, payload protocol.P2PConnectPayload) error {
	key, err := decodeP2PSessionKey(payload.SessionKey)
	if err != nil {
		return err
	}
	setupCtx, cancel := context.WithTimeout(ctx, p2pStreamSetupTimeout)
	defer cancel()
	conn, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return err
	}
	defer conn.Close()
	rendezvous, err := net.ResolveUDPAddr("udp", payload.RendezvousAddress)
	if err != nil {
		return err
	}
	result, err := registerP2PPeer(setupCtx, conn, rendezvous, payload)
	if err != nil {
		return err
	}
	if err := punchP2PStreamPeer(setupCtx, conn, result.Peer, payload.WantsPeerID); err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Time{})
	certificate, err := netx.SelfSignedCertificate()
	if err != nil {
		return err
	}
	server, err := netx.ListenQUICPacketConn(conn, netx.ServerTLSConfig(certificate), func(
		_ context.Context, request netx.AuthRequest, _ net.Addr,
	) error {
		return validateP2PServer(request, key, payload)
	})
	if err != nil {
		return err
	}
	defer server.Close()
	session, err := server.Accept(setupCtx)
	if err != nil {
		return err
	}
	defer session.Close()
	dataStream, err := session.AcceptStream(setupCtx)
	if err != nil {
		return err
	}
	if dataStream.Kind != netx.FrameData || dataStream.Initial.RequestID != payload.RequestID {
		_ = dataStream.Close()
		return errors.New("invalid p2p data stream")
	}
	proof, err := netx.P2PProof(key, payload.SessionID, payload.RequestID, netx.P2PStreamRoleAgent)
	if err != nil {
		return err
	}
	if err := netx.WriteFrame(dataStream, netx.Frame{Kind: netx.FrameAuth, Payload: []byte(proof)}); err != nil {
		return err
	}
	localAddress := net.JoinHostPort(payload.LocalHost, strconv.Itoa(payload.LocalPort))
	local, err := dialTCP(setupCtx, localAddress)
	if err != nil {
		_ = dataStream.Close()
		return err
	}
	defer local.Close()
	return a.relay(payload.TunnelID, dataStream, local)
}

func punchP2PStreamPeer(
	ctx context.Context,
	conn net.PacketConn,
	peer netx.Endpoint,
	selfID string,
) error {
	punchCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := netx.PunchHandshake(punchCtx, conn, peer, selfID); err != nil {
		return err
	}
	return nil
}

func validateP2PServer(
	request netx.AuthRequest,
	key []byte,
	payload protocol.P2PConnectPayload,
) error {
	if request.DeviceID != payload.SessionID || request.Role != netx.P2PStreamRoleServer {
		return errors.New("invalid p2p server identity")
	}
	if !netx.VerifyP2PProof(key, payload.SessionID, payload.RequestID, netx.P2PStreamRoleServer, request.Token) {
		return errors.New("invalid p2p server proof")
	}
	return nil
}
