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
	connectedAt  map[string]time.Time
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

func (r *runtimeSpy) ClientConnectedAt(id string) (time.Time, bool) {
	connectedAt, ok := r.connectedAt[id]
	return connectedAt, ok
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
	if _, err := service.AuthenticateAgent(context.Background(), cleartext); err != nil {
		t.Fatal("reset disabled a token that may be shared by another device")
	}
	if _, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "home", DeviceID: client.DeviceID,
	}); err == nil {
		t.Fatal("old token reclaimed the reset device")
	}
}

func TestTokenDisableDisconnectsBoundClient(t *testing.T) {
	store, service, router, session, runtime := managementRouter(t)
	token, _, err := service.CreateAgentToken(context.Background(), "disable-token")
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "disable", DeviceID: "disable-device",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, http.MethodPatch, "/api/tokens/"+token.ID, session,
		map[string]any{"disabled": true})
	if response.Code != http.StatusNoContent {
		t.Fatalf("disable status=%d body=%s", response.Code, response.Body.String())
	}
	if len(runtime.disconnected) != 1 || runtime.disconnected[0] != client.ID {
		t.Fatalf("bound client was not disconnected: %v", runtime.disconnected)
	}
}

func TestTokenDeleteRemovesBoundClientAndTunnel(t *testing.T) {
	store, service, router, session, runtime := managementRouter(t)
	token, _, err := service.CreateAgentToken(context.Background(), "delete-token")
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "delete", DeviceID: "delete-device",
	})
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := store.CreateTunnel(context.Background(), model.Tunnel{
		Name: "delete", ClientID: client.ID, Protocol: "tcp", LocalHost: "127.0.0.1",
		LocalPort: 80, RemotePort: 6081, Status: "running",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTunnelStatus(context.Background(), tunnel.ID, "running"); err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, http.MethodDelete, "/api/tokens/"+token.ID, session, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
	}
	if len(runtime.stopped) != 1 || runtime.stopped[0] != tunnel.ID || len(runtime.disconnected) != 1 {
		t.Fatalf("runtime cleanup missing: stopped=%v disconnected=%v", runtime.stopped, runtime.disconnected)
	}
	if _, err := store.GetClient(context.Background(), client.ID); err == nil {
		t.Fatal("bound client remains after token deletion")
	}
}

func TestClientDeleteRevokesItsDeviceIdentity(t *testing.T) {
	store, service, router, session, runtime := managementRouter(t)
	token, cleartext, err := service.CreateAgentToken(context.Background(), "client-delete")
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "delete", DeviceID: "client-delete-device",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, http.MethodDelete, "/api/clients/"+client.ID, session, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := service.AuthenticateAgent(context.Background(), cleartext); err != nil {
		t.Fatal("deleting one client disabled its reusable token")
	}
	if len(runtime.disconnected) != 1 || runtime.disconnected[0] != client.ID {
		t.Fatalf("client was not disconnected: %v", runtime.disconnected)
	}
}

func TestClientResetDisconnectsOnlyTargetWithSharedToken(t *testing.T) {
	store, service, router, session, runtime := managementRouter(t)
	token, cleartext, err := service.CreateAgentToken(context.Background(), "shared-reset")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "first", DeviceID: "reset-first",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "second", DeviceID: "reset-second",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := requestJSON(t, router, http.MethodPost,
		"/api/clients/"+first.ID+"/reset-token", session, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("reset status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := service.AuthenticateAgent(context.Background(), cleartext); err != nil {
		t.Fatal("shared token was disabled during target reset")
	}
	if len(runtime.disconnected) != 1 || runtime.disconnected[0] != first.ID {
		t.Fatalf("wrong clients disconnected: target=%s other=%s got=%v", first.ID, second.ID, runtime.disconnected)
	}
}

func TestClientDetailIncludesConnectionAndTraffic(t *testing.T) {
	store, service, router, session, runtime := managementRouter(t)
	token, _, err := service.CreateAgentToken(context.Background(), "detail-token")
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "detail", DeviceID: "detail-device", IP: "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := store.CreateTunnel(context.Background(), model.Tunnel{
		Name: "detail-tunnel", ClientID: client.ID, Protocol: "tcp",
		LocalHost: "127.0.0.1", LocalPort: 80, RemotePort: 6080,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordTraffic(context.Background(), tunnel.ID, 100, 250); err != nil {
		t.Fatal(err)
	}
	runtime.connectedAt = map[string]time.Time{client.ID: time.Now().Add(-5 * time.Minute)}
	response := requestJSON(t, router, http.MethodGet, "/api/clients/"+client.ID, session, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", response.Code, response.Body.String())
	}
	var detail struct {
		ConnectedSeconds int64          `json:"connected_seconds"`
		Traffic          clientTraffic  `json:"traffic"`
		Tunnels          []model.Tunnel `json:"tunnels"`
	}
	decodeJSON(t, response, &detail)
	if detail.ConnectedSeconds < 299 || detail.Traffic.Today.Upload != 100 ||
		detail.Traffic.Month.Download != 250 || len(detail.Tunnels) != 1 {
		t.Fatalf("unexpected client detail: %+v", detail)
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
