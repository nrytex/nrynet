package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreLoadsLegacyDesktopConfig(t *testing.T) {
	directory := t.TempDir()
	legacyPath := filepath.Join(directory, "NAT-Link", "desktop.yaml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o750); err != nil {
		t.Fatal(err)
	}
	data := []byte("client:\n  server_url: wss://legacy.example/agent/connect\n  token: legacy-token\nauto_start: true\n")
	if err := os.WriteFile(legacyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store := &fileStore{path: filepath.Join(directory, "Nrynet", "desktop.yaml"), legacyPath: legacyPath}
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerURL != "wss://legacy.example/agent/connect" || cfg.Token != "legacy-token" || !cfg.AutoStart {
		t.Fatalf("legacy config was not loaded: %#v", cfg)
	}
}
