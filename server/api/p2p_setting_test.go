package api

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/auth"
	"github.com/nrytex/nrynet/internal/storage"
)

type p2pSettingApplier struct {
	key   string
	value string
}

func (a *p2pSettingApplier) ApplySetting(_ context.Context, key, value string) error {
	a.key = key
	a.value = value
	return nil
}

func TestP2PSettingPersistsAndAppliesAtRuntime(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err := auth.New(ctx, store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bootstrap(ctx, "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	applier := &p2pSettingApplier{}
	router := NewRouterWithOptions(store, service, time.Now(), RouterOptions{
		Settings:       []SettingItem{{Key: "server.p2p_enabled", Value: true, Mutable: true}},
		SettingApplier: applier,
	})
	session := loginForSettings(t, router)
	response := requestJSON(t, router, http.MethodPatch, "/api/settings/server.p2p_enabled", session, map[string]any{"value": false})
	if response.Code != http.StatusOK {
		t.Fatalf("setting update status=%d body=%s", response.Code, response.Body.String())
	}
	value, err := store.GetSetting(ctx, "config.server.p2p_enabled")
	if err != nil || value != "false" {
		t.Fatalf("stored value=%q err=%v", value, err)
	}
	if applier.key != "server.p2p_enabled" || applier.value != "false" {
		t.Fatalf("runtime apply key=%q value=%q", applier.key, applier.value)
	}
}
