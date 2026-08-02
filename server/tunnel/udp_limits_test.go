package tunnel

import (
	"net"
	"testing"

	"github.com/nat-link/nat-link/internal/model"
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
