package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/nrytex/nrynet/internal/protocol"
)

const visitorGatherTimeout = 15 * time.Second

func (a *Agent) handleVisitorWebRTC(ctx context.Context, conn controlConn, message protocol.ControlMessage) error {
	payload, err := protocol.DecodePayload[protocol.VisitorWebRTCSignalPayload](message)
	if err != nil {
		return err
	}
	if err := validateVisitorOffer(message, payload); err != nil {
		return a.sendVisitorSignalError(conn, message, payload.SessionID, err)
	}
	peer, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: visitorICEServers(payload.ICEServers),
	})
	if err != nil {
		return a.sendVisitorSignalError(conn, message, payload.SessionID, err)
	}
	defer peer.Close()

	visitorCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	closed := make(chan struct{})
	var closeOnce sync.Once
	closePeer := func() {
		closeOnce.Do(func() {
			cancel()
			close(closed)
		})
	}
	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		defer a.recoverWorker("visitor WebRTC state change")
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			closePeer()
		}
	})
	peer.OnDataChannel(func(channel *webrtc.DataChannel) {
		defer a.recoverWorker("visitor WebRTC data channel binding")
		a.bindVisitorDataChannel(visitorCtx, channel, payload.LocalHost, payload.LocalPort, closePeer)
	})
	if err := peer.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  payload.SDP,
	}); err != nil {
		return a.sendVisitorSignalError(conn, message, payload.SessionID, err)
	}
	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		return a.sendVisitorSignalError(conn, message, payload.SessionID, err)
	}
	if err := peer.SetLocalDescription(answer); err != nil {
		return a.sendVisitorSignalError(conn, message, payload.SessionID, err)
	}
	if err := waitVisitorGathering(visitorCtx, peer); err != nil {
		return a.sendVisitorSignalError(conn, message, payload.SessionID, err)
	}
	local := peer.LocalDescription()
	if local == nil || local.SDP == "" {
		return a.sendVisitorSignalError(conn, message, payload.SessionID, fmt.Errorf("WebRTC answer is empty"))
	}
	answerMessage, err := protocol.NewMessage(protocol.TypeVisitorWebRTC, message.RequestID, message.TunnelID,
		protocol.VisitorWebRTCSignalPayload{Kind: "answer", SessionID: payload.SessionID, SDP: local.SDP})
	if err != nil {
		return err
	}
	if err := a.writeControl(conn, answerMessage); err != nil {
		return fmt.Errorf("send WebRTC answer: %w", err)
	}
	select {
	case <-ctx.Done():
	case <-closed:
	}
	return nil
}

func validateVisitorOffer(message protocol.ControlMessage, payload protocol.VisitorWebRTCSignalPayload) error {
	if message.TunnelID == "" || payload.SessionID == "" || payload.SDP == "" {
		return fmt.Errorf("visitor WebRTC offer is incomplete")
	}
	if payload.LocalHost == "" || payload.LocalPort < 1 || payload.LocalPort > 65535 {
		return fmt.Errorf("visitor WebRTC local service address is invalid")
	}
	return nil
}

func visitorICEServers(values []string) []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		servers = append(servers, webrtc.ICEServer{URLs: []string{value}})
	}
	return servers
}

func waitVisitorGathering(ctx context.Context, peer *webrtc.PeerConnection) error {
	complete := webrtc.GatheringCompletePromise(peer)
	timer := time.NewTimer(visitorGatherTimeout)
	defer timer.Stop()
	select {
	case <-complete:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return fmt.Errorf("WebRTC ICE gathering timed out")
	}
}

func (a *Agent) bindVisitorDataChannel(
	ctx context.Context,
	channel *webrtc.DataChannel,
	localHost string,
	localPort int,
	closePeer func(),
) {
	dataSession := newVisitorDataSession(a, ctx)
	dataSession.configureChannel(channel)
	go func() {
		defer a.recoverWorker("visitor data session cleanup")
		<-ctx.Done()
		dataSession.close()
	}()
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		defer a.recoverWorker("visitor data channel")
		dataSession.handle(channel, localHost, localPort, message.Data)
	})
	channel.OnClose(func() {
		defer a.recoverWorker("visitor data channel close")
		dataSession.close()
		closePeer()
	})
}

func (a *Agent) sendVisitorSignalError(
	conn controlConn,
	message protocol.ControlMessage,
	sessionID string,
	err error,
) error {
	errorMessage, messageErr := protocol.NewMessage(protocol.TypeVisitorWebRTC, message.RequestID, message.TunnelID,
		protocol.VisitorWebRTCSignalPayload{Kind: "error", SessionID: sessionID, Error: err.Error()})
	if messageErr != nil {
		return messageErr
	}
	if writeErr := a.writeControl(conn, errorMessage); writeErr != nil {
		return fmt.Errorf("send WebRTC error: %w", writeErr)
	}
	return err
}
