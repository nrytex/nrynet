package api

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/auth"
	"github.com/nrytex/nrynet/internal/storage"
)

func TestTransportEndpointsRequireSessionAndCallManager(t *testing.T) {
	router, manager, session := transportRouter(t)
	unauthorized := requestJSON(t, router, http.MethodGet, "/api/transport", "", nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("transport status without session=%d", unauthorized.Code)
	}
	issued := requestJSON(t, router, http.MethodPost, "/api/transport/certificates", session,
		map[string]any{"domain": "nrynet.example.com", "email": "admin@example.com"})
	if issued.Code != http.StatusAccepted {
		t.Fatalf("certificate status=%d body=%s", issued.Code, issued.Body.String())
	}
	if manager.lastRequest.Domain != "nrynet.example.com" {
		t.Fatalf("manager request not called: %+v", manager.lastRequest)
	}
	tls := requestJSON(t, router, http.MethodPatch, "/api/transport/tls", session, map[string]any{"enabled": true})
	if tls.Code != http.StatusOK || !manager.tlsEnabled {
		t.Fatalf("tls update status=%d enabled=%v", tls.Code, manager.tlsEnabled)
	}
	plain := requestJSON(t, router, http.MethodPatch, "/api/transport/plain", session, map[string]any{"enabled": false})
	if plain.Code != http.StatusOK || manager.plainEnabled {
		t.Fatalf("plain update status=%d enabled=%v", plain.Code, manager.plainEnabled)
	}
}

func TestTransportErrorsAreFriendlyChinese(t *testing.T) {
	router, manager, session := transportRouter(t)
	manager.err = errors.New("certbot not found")
	response := requestJSON(t, router, http.MethodPost, "/api/transport/certificates", session,
		map[string]any{"domain": "nrynet.example.com", "email": "admin@example.com"})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); body == "" || !strings.Contains(body, "Certbot") {
		t.Fatalf("unexpected friendly body=%s", body)
	}
}

func transportRouter(t *testing.T) (http.Handler, *fakeTransportManager, string) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "transport.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := auth.New(ctx, store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bootstrap(ctx, "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	manager := &fakeTransportManager{status: TransportStatus{
		Plain:        TransportEndpoint{Enabled: true, ControlURL: "http://127.0.0.1:7000"},
		TLS:          TransportEndpoint{Enabled: false},
		Capabilities: TransportCapabilities{CertbotAvailable: true, HotReload: true},
	}}
	router := NewRouterWithOptions(store, service, time.Now(), RouterOptions{Transport: manager})
	return router, manager, loginForSettings(t, router)
}

type fakeTransportManager struct {
	status       TransportStatus
	lastRequest  CertificateRequest
	tlsEnabled   bool
	plainEnabled bool
	err          error
}

func (f *fakeTransportManager) Status(context.Context) (TransportStatus, error) {
	return f.status, f.err
}

func (f *fakeTransportManager) RequestCertificate(_ context.Context, request CertificateRequest) (TransportStatus, error) {
	f.lastRequest = request
	return f.status, f.err
}

func (f *fakeTransportManager) SetTLSEnabled(_ context.Context, enabled bool) (TransportStatus, error) {
	f.tlsEnabled = enabled
	f.status.TLS.Enabled = enabled
	return f.status, f.err
}

func (f *fakeTransportManager) SetPlainEnabled(_ context.Context, enabled bool) (TransportStatus, error) {
	f.plainEnabled = enabled
	f.status.Plain.Enabled = enabled
	return f.status, f.err
}
