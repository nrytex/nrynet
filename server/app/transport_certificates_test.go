package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nrytex/nrynet/server/api"
	"github.com/nrytex/nrynet/server/certbothelper"
)

func TestCertificateQueueAndSuccessHotEnableTLS(t *testing.T) {
	ctx := context.Background()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	cfg.Server.TLS.Enabled = false
	cfg.Server.TLS.CertFile = ""
	cfg.Server.TLS.KeyFile = ""
	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(ctx)

	controller := application.TransportController()
	if err := os.MkdirAll(filepath.Dir(controller.certbot.ReadyPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controller.certbot.ReadyPath, nil, 0o640); err != nil {
		t.Fatal(err)
	}
	status, err := controller.RequestCertificate(ctx, api.CertificateRequest{
		Domain: "relay.example.com", Email: "admin@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Certificate == nil || status.Certificate.Status != "pending" {
		t.Fatalf("certificate status=%+v", status.Certificate)
	}

	writeCertbotSuccess(t, controller.certbot, certFile, keyFile)
	controller.applyCertificateUpdates()
	status, err = controller.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.TLS.Enabled || status.Certificate == nil || status.Certificate.Domain != "relay.example.com" {
		t.Fatalf("hot certificate status=%+v", status)
	}
	value, err := application.store.GetSetting(ctx, "config.server.tls.enabled")
	if err != nil || value != "true" {
		t.Fatalf("persisted TLS enabled=%q err=%v", value, err)
	}
}

func TestDisabledTLSStaysDisabledAcrossOldCertbotStatus(t *testing.T) {
	ctx := context.Background()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer application.Shutdown(ctx)
	controller := application.TransportController()
	writeCertbotSuccess(t, controller.certbot, certFile, keyFile)
	controller.applyCertificateUpdates()
	if _, err := controller.SetTLSEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	controller.applyCertificateUpdates()
	status, _ := controller.Status(ctx)
	if status.TLS.Enabled {
		t.Fatal("an already-applied certificate job re-enabled TLS")
	}
}

func writeCertbotSuccess(t *testing.T, options certbothelper.Options, certFile, keyFile string) {
	t.Helper()
	status := certbothelper.Status{
		Action: "issue", Domain: "relay.example.com", Email: "admin@example.com",
		State: "success", Updated: time.Now(), CertFile: certFile, KeyFile: keyFile,
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(options.StatusPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.StatusPath, data, 0o640); err != nil {
		t.Fatal(err)
	}
}
