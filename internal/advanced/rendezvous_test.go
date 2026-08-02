package advanced

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestRendezvousCoordinatesUDPHolePunching(t *testing.T) {
	server, err := ListenRendezvous("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = server.Serve(ctx) }()

	left := listenUDP(t)
	right := listenUDP(t)
	defer left.Close()
	defer right.Close()
	leftResult, rightResult := exchangeEndpoints(t, ctx, server.Addr(), left, right)
	if leftResult.Peer.Port == 0 || rightResult.Peer.Port == 0 {
		t.Fatalf("missing peer endpoints: %#v %#v", leftResult, rightResult)
	}
	done := make(chan Endpoint, 1)
	go func() {
		endpoint, _ := AwaitPunch(ctx, right)
		done <- endpoint
	}()
	if err := sendPunch(left, leftResult.Peer, "left"); err != nil {
		t.Fatal(err)
	}
	punched := <-done
	if punched.Port == 0 {
		t.Fatal("expected punched endpoint")
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.peers) != 0 {
		t.Fatalf("paired rendezvous entries were not released: %d", len(server.peers))
	}
}

func TestPunchHandshakeWaitsForReciprocalAcknowledgement(t *testing.T) {
	left := listenUDP(t)
	right := listenUDP(t)
	defer left.Close()
	defer right.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	leftPeer := endpointFromAddr(right.LocalAddr())
	rightPeer := endpointFromAddr(left.LocalAddr())
	errors := make(chan error, 2)
	go func() { errors <- PunchHandshake(ctx, left, leftPeer, "left") }()
	go func() { errors <- PunchHandshake(ctx, right, rightPeer, "right") }()
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
}

func TestRendezvousCapsUnmatchedRegistrations(t *testing.T) {
	conn := listenUDP(t)
	defer conn.Close()
	server := NewRendezvousServer(conn)
	server.max = 2
	for _, peerID := range []string{"a", "b", "c"} {
		server.register(RendezvousPacket{
			Type: PacketRegister, SessionID: "session-" + peerID,
			PeerID: peerID, WantsPeerID: "missing",
		}, conn.LocalAddr())
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.peers) != 2 {
		t.Fatalf("peer cap not enforced: %d", len(server.peers))
	}
}

func sendPunch(conn net.PacketConn, peer Endpoint, peerID string) error {
	addr, err := peer.UDPAddr()
	if err != nil {
		return err
	}
	data, err := json.Marshal(RendezvousPacket{Type: PacketPunch, PeerID: peerID})
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(data, addr)
	return err
}

func exchangeEndpoints(
	t *testing.T,
	ctx context.Context,
	serverAddr net.Addr,
	left net.PacketConn,
	right net.PacketConn,
) (RendezvousResult, RendezvousResult) {
	t.Helper()
	leftCh := make(chan RendezvousResult, 1)
	rightCh := make(chan RendezvousResult, 1)
	errCh := make(chan error, 2)
	go rendezvousPeer(ctx, left, serverAddr, "left", "right", leftCh, errCh)
	go rendezvousPeer(ctx, right, serverAddr, "right", "left", rightCh, errCh)
	select {
	case err := <-errCh:
		t.Fatal(err)
	case leftResult := <-leftCh:
		return leftResult, <-rightCh
	}
	return RendezvousResult{}, RendezvousResult{}
}

func rendezvousPeer(
	ctx context.Context,
	conn net.PacketConn,
	server net.Addr,
	peerID string,
	wantsPeerID string,
	resultCh chan<- RendezvousResult,
	errCh chan<- error,
) {
	result, err := Rendezvous(ctx, conn, server, RendezvousPacket{
		Type:        PacketRegister,
		SessionID:   "session-1",
		PeerID:      peerID,
		WantsPeerID: wantsPeerID,
	})
	if err != nil {
		errCh <- err
		return
	}
	resultCh <- result
}

func listenUDP(t *testing.T) net.PacketConn {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return conn
}
