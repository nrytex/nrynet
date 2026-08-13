package tunnel

import (
	"net"
	"testing"

	"github.com/nrytex/nrynet/internal/model"
)

func TestUDPRuntimeAllowsHighP2PConcurrency(t *testing.T) {
	runtime := newUDPRuntime(&Manager{}, model.Tunnel{}, nil)
	const sessions = 4096
	for index := 0; index < sessions; index++ {
		if !runtime.acquireP2P() {
			t.Fatalf("p2p socket %d was unexpectedly rejected", index)
		}
	}
	if runtime.p2pCount != sessions {
		t.Fatalf("p2p count=%d want=%d", runtime.p2pCount, sessions)
	}
}

func TestUDPRuntimeStillBoundsVisitorSessions(t *testing.T) {
	runtime := newUDPRuntime(&Manager{}, model.Tunnel{}, nil)
	for index := 0; index < udpMaxSessions; index++ {
		addr := &net.UDPAddr{IP: net.IPv4(127, byte(index>>8), byte(index), 1), Port: index + 1}
		if runtime.session(addr) == nil {
			t.Fatalf("session %d was rejected before the configured session bound", index)
		}
	}
	if runtime.session(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 65500}) != nil {
		t.Fatal("visitor session above the configured bound was accepted")
	}
}
