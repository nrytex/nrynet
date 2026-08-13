package tunnel

import (
	"net"
	"testing"

	"github.com/nrytex/nrynet/internal/model"
)

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
		t.Fatalf("closed session consumed a P2P slot: %d", runtime.p2pCount)
	}
}
