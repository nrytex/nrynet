package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/storage"
	"github.com/nrytex/nrynet/server/api"
)

func TestApplyStoredSettingsSeedsAutoSubdomainDefaults(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := config.Config{Server: config.ServerConfig{
		HeartbeatText: "45s",
		AutoSubdomain: config.AutoSubdomainConfig{
			Enabled:    true,
			BaseDomain: "Tunnels.Example.COM.",
		},
	}}
	if err := applyStoredSettings(ctx, store, &cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.AutoSubdomainConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Enabled || loaded.BaseDomain != "tunnels.example.com" {
		t.Fatalf("stored auto-subdomain config=%+v", loaded)
	}
}

func TestApplyStoredSettingsKeepsStoredAutoSubdomainOverride(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetSetting(ctx, "config.server.auto_subdomain.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(ctx, "config.server.auto_subdomain.base_domain", "stored.example.com"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Server: config.ServerConfig{
		HeartbeatText: "45s",
		AutoSubdomain: config.AutoSubdomainConfig{
			Enabled:    true,
			BaseDomain: "yaml.example.com",
		},
	}}
	if err := applyStoredSettings(ctx, store, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.AutoSubdomain.Enabled || cfg.Server.AutoSubdomain.BaseDomain != "stored.example.com" {
		t.Fatalf("runtime auto-subdomain config=%+v", cfg.Server.AutoSubdomain)
	}
}

func TestApplyStoredSettingsRejectsEnabledAutoSubdomainWithoutBase(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := config.Config{Server: config.ServerConfig{
		HeartbeatText: "45s",
		AutoSubdomain: config.AutoSubdomainConfig{
			Enabled: true,
		},
	}}
	if err := applyStoredSettings(ctx, store, &cfg); err == nil {
		t.Fatal("enabled auto-subdomain without base domain was accepted")
	}
}

func TestTransportControllerUpdatesAutoSubdomainSettings(t *testing.T) {
	ctx := context.Background()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(ctx)
	enabled := true
	status, err := application.TransportController().SetAutoSubdomain(ctx, api.AutoSubdomainRequest{
		Enabled:    &enabled,
		BaseDomain: "Tunnels.Example.COM.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !status.AutoSubdomain.Enabled || status.AutoSubdomain.BaseDomain != "tunnels.example.com" {
		t.Fatalf("transport auto-subdomain status=%+v", status.AutoSubdomain)
	}
	stored, err := application.store.AutoSubdomainConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Enabled || stored.BaseDomain != "tunnels.example.com" {
		t.Fatalf("stored auto-subdomain config=%+v", stored)
	}
	settings := safeSettings(application.config)
	for _, item := range settings {
		if item.Key == "server.auto_subdomain.enabled" || item.Key == "server.auto_subdomain.base_domain" {
			t.Fatalf("auto-subdomain should not be exposed through generic settings: %s", item.Key)
		}
	}
}

func TestTransportControllerRejectsEnabledAutoSubdomainWithoutBase(t *testing.T) {
	ctx := context.Background()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(ctx)
	enabled := true
	if _, err := application.TransportController().SetAutoSubdomain(ctx, api.AutoSubdomainRequest{Enabled: &enabled}); err == nil {
		t.Fatal("enabled auto-subdomain without base domain was accepted")
	}
	if value, err := application.store.GetSetting(ctx, "config.server.auto_subdomain.enabled"); err != nil || value != "false" {
		t.Fatalf("stored enabled after reject value=%q err=%v", value, err)
	}
	if value, err := application.store.GetSetting(ctx, "config.server.auto_subdomain.base_domain"); err != nil && !errors.Is(err, sql.ErrNoRows) || value != "" {
		t.Fatalf("stored base after reject value=%q err=%v", value, err)
	}
}
