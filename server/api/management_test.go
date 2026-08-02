package api

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/storage"
)

type runtimeSpy struct {
	store        *storage.Store
	started      []string
	stopped      []string
	disconnected []string
}

func (r *runtimeSpy) StartTunnel(ctx context.Context, id string) error {
	r.started = append(r.started, id)
	return r.store.SetTunnelStatus(ctx, id, "running")
}

func (r *runtimeSpy) StopTunnel(ctx context.Context, id string) error {
	r.stopped = append(r.stopped, id)
	return r.store.SetTunnelStatus(ctx, id, "stopped")
}

func (r *runtimeSpy) SyncClient(context.Context, string) error { return nil }

func (r *runtimeSpy) DisconnectClient(id string) {
	r.disconnected = append(r.disconnected, id)
}

func TestClientManagementAndTokenReset(t *testing.T) {
	store, service, router, session, runtime := managementRouter(t)
	token, cleartext, err := service.CreateAgentToken(context.Background(), "client-token")
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "home", DeviceID: "home-device", IP: "127.0.0.1", OS: "linux", Version: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	disabled := requestJSON(t, router, http.MethodPatch, "/api/clients/"+client.ID, session,
		map[string]any{"name": "home-renamed", "disabled": true})
	if disabled.Code != http.StatusNoContent || len(runtime.disconnected) != 1 {
		t.Fatalf("disable status=%d disconnected=%v", disabled.Code, runtime.disconnected)
	}
	reset := requestJSON(t, router, http.MethodPost, "/api/clients/"+client.ID+"/reset-token", session, nil)
	if reset.Code != http.StatusCreated {
		t.Fatalf("reset status=%d body=%s", reset.Code, reset.Body.String())
	}
	if _, err := service.AuthenticateAgent(context.Background(), cleartext); err == nil {
		t.Fatal("old token remained valid after reset")
	}
}

func TestTunnelCRUDCallsRuntime(t *testing.T) {
	store, service, router, session, runtime := managementRouter(t)
	token, _, err := service.CreateAgentToken(context.Background(), "client-token")
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "home", DeviceID: "tunnel-device", IP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	create := requestJSON(t, router, http.MethodPost, "/api/tunnels", session, map[string]any{
		"name": "ssh", "protocol": "tcp", "client_id": client.ID,
		"local_host": "127.0.0.1", "local_port": 22, "remote_port": 6022, "status": "running",
	})
	if create.Code != http.StatusCreated || len(runtime.started) != 1 {
		t.Fatalf("create status=%d body=%s started=%v", create.Code, create.Body.String(), runtime.started)
	}
	var tunnel model.Tunnel
	decodeJSON(t, create, &tunnel)
	update := requestJSON(t, router, http.MethodPut, "/api/tunnels/"+tunnel.ID, session, map[string]any{
		"name": "ssh-updated", "protocol": "tcp", "client_id": client.ID,
		"local_host": "127.0.0.1", "local_port": 2222, "remote_port": 6022,
	})
	if update.Code != http.StatusOK || len(runtime.stopped) == 0 || len(runtime.started) != 2 {
		t.Fatalf("update status=%d stopped=%v started=%v", update.Code, runtime.stopped, runtime.started)
	}
	deleted := requestJSON(t, router, http.MethodDelete, "/api/tunnels/"+tunnel.ID, session, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	logs := requestJSON(t, router, http.MethodGet, "/api/logs?keyword=Tunnel", session, nil)
	if logs.Code != http.StatusOK || !containsText(logs.Body.String(), "tunnel.created") {
		t.Fatalf("logs status=%d body=%s", logs.Code, logs.Body.String())
	}
	download := requestJSON(t, router, http.MethodGet, "/api/logs/download", session, nil)
	if download.Code != http.StatusOK || download.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("download status=%d content-type=%q", download.Code, download.Header().Get("Content-Type"))
	}
	cleared := requestJSON(t, router, http.MethodDelete, "/api/logs", session, nil)
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear logs status=%d body=%s", cleared.Code, cleared.Body.String())
	}
}

func containsText(value, needle string) bool {
	return strings.Contains(value, needle)
}

func managementRouter(t *testing.T) (*storage.Store, *auth.Service, http.Handler, string, *runtimeSpy) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "management.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := auth.New(context.Background(), store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bootstrap(context.Background(), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	runtime := &runtimeSpy{store: store}
	router := NewRouter(store, service, time.Now(), runtime)
	login := requestJSON(t, router, http.MethodPost, "/api/auth/login", "", map[string]any{
		"username": "admin", "password": "test-password",
	})
	var session struct {
		Token string `json:"token"`
	}
	decodeJSON(t, login, &session)
	return store, service, router, session.Token, runtime
}
