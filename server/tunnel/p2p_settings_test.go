package tunnel

import (
	"context"
	"testing"
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
