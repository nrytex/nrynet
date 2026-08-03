package tunnel

import (
	"net"
	"testing"

	"github.com/nrytex/nrynet/internal/model"
)

func TestUDPRuntimeBoundsSessionsAndP2PSockets(t *testing.T) {
	manager := &Manager{}
	runtime := newUDPRuntime(manager, model.Tunnel{}, nil)
	for index := 0; index < udpMaxSessions; index++ {
		addr := &net.UDPAddr{IP: net.IPv4(127, byte(index>>8), byte(index), 1), Port: index + 1}
		if runtime.session(addr) == nil {
			t.Fatalf("session %d was rejected before the limit", index)
		}
	}
	if runtime.session(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 65500}) != nil {
		t.Fatal("session above the limit was accepted")
	}
	for index := 0; index < udpMaxP2PSessions; index++ {
		if !runtime.acquireP2P() {
			t.Fatalf("p2p socket %d was rejected before the limit", index)
		}
	}
	if runtime.acquireP2P() {
		t.Fatal("p2p socket above the limit was accepted")
	}
	runtime.releaseP2P()
	if !runtime.acquireP2P() {
		t.Fatal("released p2p capacity was not reusable")
	}
}

func TestDisconnectUDPClientClearsOnlyItsSessions(t *testing.T) {
	manager := &Manager{udpRuntimes: make(map[string]*udpRuntime)}
	first := newUDPRuntime(manager, model.Tunnel{ID: "first", ClientID: "client-a"}, nil)
	second := newUDPRuntime(manager, model.Tunnel{ID: "second", ClientID: "client-b"}, nil)
	manager.udpRuntimes[first.tunnel.ID] = first
	manager.udpRuntimes[second.tunnel.ID] = second
	first.session(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1001})
	second.session(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1002})

	manager.disconnectUDPClient("client-a")
	if len(first.sessions) != 0 || len(second.sessions) != 1 {
		t.Fatalf("unexpected session counts: first=%d second=%d", len(first.sessions), len(second.sessions))
	}
}

func TestClosedUDPSessionCannotOpenP2P(t *testing.T) {
	tunnel := model.Tunnel{ID: "closed", ClientID: "client"}
	manager := &Manager{
		udpRuntimes: make(map[string]*udpRuntime),
		rdvAddress:  "127.0.0.1:7003",
	}
	runtime := newUDPRuntime(manager, tunnel, nil)
	manager.udpRuntimes[tunnel.ID] = runtime
	session := runtime.session(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1003})
	session.closeP2P()
	if manager.tryP2PUDPPacket(tunnel, session, []byte("blocked")) {
		t.Fatal("closed UDP session opened a P2P path")
	}
	if runtime.p2pCount != 0 {
		t.Fatalf("closed session consumed P2P capacity: %d", runtime.p2pCount)
	}
}
