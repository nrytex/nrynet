package app

import (
	"context"
	"testing"

	"github.com/nrytex/nrynet/internal/storage"
)

func TestPartialPlaintextPairIsRejected(t *testing.T) {
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	cfg.Server.PlainDataListen = ""
	application, _, err := New(context.Background(), cfg)
	if application != nil {
		_ = application.Shutdown(context.Background())
	}
	if err == nil {
		t.Fatal("enabled plaintext listeners with a partial address pair were accepted")
	}
}

func TestPlaintextDisabledDoesNotBindConfiguredPorts(t *testing.T) {
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	cfg.Server.PlainEnabled = false
	plainListen := cfg.Server.PlainListen
	plainDataListen := cfg.Server.PlainDataListen
	application, _, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(context.Background())
	if application.plain != nil || application.plainData != nil {
		t.Fatal("disabled plaintext listeners should not bind")
	}
	assertCanListen(t, "tcp", plainListen)
	assertCanListen(t, "tcp", plainDataListen)
}

func TestStoredPlainEnabledControlsNextStartup(t *testing.T) {
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	cfg.Server.PlainEnabled = false
	persistAppSetting(t, cfg.Server.Database, "config.server.plain_enabled", "true")
	application, _, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(context.Background())
	if application.plain == nil || application.plainData == nil {
		t.Fatal("stored server.plain_enabled=true should bind plaintext listeners on startup")
	}
}

func TestStoredPlainDisabledControlsNextStartup(t *testing.T) {
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	persistAppSetting(t, cfg.Server.Database, "config.server.plain_enabled", "false")
	application, _, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(context.Background())
	if application.plain != nil || application.plainData != nil {
		t.Fatal("stored server.plain_enabled=false should not bind plaintext listeners on startup")
	}
}

func persistAppSetting(t *testing.T, database, key, value string) {
	t.Helper()
	store, err := storage.Open(database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetSetting(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
}
