package tunnel

import (
	"context"
	"testing"
	"time"
)

func TestApplyP2PSettingUpdatesRuntime(t *testing.T) {
	manager := &Manager{p2pEnabled: true}
	if err := manager.ApplySetting(context.Background(), "server.p2p_enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if manager.p2pEnabledNow() {
		t.Fatal("p2p runtime remained enabled")
	}
	if err := manager.ApplySetting(context.Background(), "server.p2p_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if !manager.p2pEnabledNow() {
		t.Fatal("p2p runtime remained disabled")
	}
}

func TestP2PStreamFailureDefersNextAttempt(t *testing.T) {
	manager := &Manager{p2pRetryAt: make(map[string]time.Time)}
	manager.deferP2PRetry("tunnel-1")
	if manager.p2pRetryAllowed("tunnel-1") {
		t.Fatal("P2P retry was not deferred after a stream failure")
	}
}
