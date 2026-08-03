package advanced

import (
	"context"
	"net"
	"testing"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
)

func TestPeerConnectorFallsBackToRelayWhenPunchTimesOut(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	peer := netx.Endpoint{IP: "127.0.0.1", Port: unusedUDPPort(t)}
	connector := PeerConnector{Timeout: 20 * time.Millisecond}
	result := connector.PunchOrRelay(context.Background(), conn, peer, "left")
	if !result.UseRelay || result.RelayReason == "" {
		t.Fatalf("expected relay fallback, got %#v", result)
	}
}

func TestSendDirectPayloadUsesDiscoveredPacketConn(t *testing.T) {
	left := listenPacket(t)
	right := listenPacket(t)
	defer left.Close()
	defer right.Close()
	done := make(chan []byte, 1)
	go echoOneDatagram(right, done)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	peer := endpointFromAddr(right.LocalAddr())
	if err := SendDirectPayload(ctx, left, peer, []byte("direct")); err != nil {
		t.Fatal(err)
	}
	if got := string(<-done); got != "direct" {
		t.Fatalf("direct payload=%q", got)
	}
}

func unusedUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func listenPacket(t *testing.T) net.PacketConn {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func echoOneDatagram(conn net.PacketConn, done chan<- []byte) {
	buffer := make([]byte, 1024)
	n, addr, err := conn.ReadFrom(buffer)
	if err != nil {
		return
	}
	done <- append([]byte(nil), buffer[:n]...)
	_, _ = conn.WriteTo([]byte("ack"), addr)
}

func endpointFromAddr(addr net.Addr) netx.Endpoint {
	udpAddr := addr.(*net.UDPAddr)
	return netx.Endpoint{IP: udpAddr.IP.String(), Port: udpAddr.Port}
}
