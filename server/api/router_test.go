package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/auth"
	"github.com/nrytex/nrynet/internal/storage"
)

func TestAuthenticationAndTokenLifecycle(t *testing.T) {
	router, closeStore := testRouter(t)
	defer closeStore()
	login := requestJSON(t, router, http.MethodPost, "/api/auth/login", "", map[string]any{
		"username": "admin", "password": "test-password",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	var session struct {
		Token string `json:"token"`
	}
	decodeJSON(t, login, &session)
	changed := requestJSON(t, router, http.MethodPost, "/api/auth/password", session.Token, map[string]any{
		"current_password": "test-password", "new_password": "a-new-password-123",
	})
	if changed.Code != http.StatusNoContent {
		t.Fatalf("password change status=%d body=%s", changed.Code, changed.Body.String())
	}
	newLogin := requestJSON(t, router, http.MethodPost, "/api/auth/login", "", map[string]any{
		"username": "admin", "password": "a-new-password-123",
	})
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new password login status=%d", newLogin.Code)
	}
	created := requestJSON(t, router, http.MethodPost, "/api/tokens", session.Token, map[string]any{"name": "home"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var token struct {
		Token struct {
			ID string `json:"id"`
		} `json:"token"`
		Value string `json:"value"`
	}
	decodeJSON(t, created, &token)
	if token.Token.ID == "" || token.Value == "" {
		t.Fatalf("cleartext token returned incorrectly: %+v", token)
	}
	listed := requestJSON(t, router, http.MethodGet, "/api/tokens", session.Token, nil)
	if listed.Code != http.StatusOK || bytes.Contains(listed.Body.Bytes(), []byte(token.Value)) {
		t.Fatalf("list leaked cleartext token: %s", listed.Body.String())
	}
	disabled := requestJSON(t, router, http.MethodPatch, "/api/tokens/"+token.Token.ID,
		session.Token, map[string]any{"disabled": true})
	if disabled.Code != http.StatusNoContent {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
}

func TestCreatedTokenIncludesServerCertificatePin(t *testing.T) {
	store, service, session := tokenRouterDependencies(t)
	pin := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	router := NewRouterWithOptions(store, service, time.Now(), RouterOptions{CertificatePin: pin})
	created := requestJSON(t, router, http.MethodPost, "/api/tokens", session,
		map[string]any{"name": "pinned"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var response struct {
		Value string `json:"value"`
	}
	decodeJSON(t, created, &response)
	if !bytes.Contains([]byte(response.Value), []byte(".spki-sha256-"+pin)) {
		t.Fatalf("created token does not include certificate pin: %q", response.Value)
	}
	if _, err := service.AuthenticateAgent(context.Background(), response.Value); err != nil {
		t.Fatalf("server rejected pinned token: %v", err)
	}
}

func TestSettingsUpdatePersistsRestartOverride(t *testing.T) {
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
	router := NewRouterWithOptions(store, service, time.Now(), RouterOptions{Settings: []SettingItem{
		{Key: "server.listen", Value: "0.0.0.0:7000", Mutable: true},
	}})
	login := requestJSON(t, router, http.MethodPost, "/api/auth/login", "", map[string]any{
		"username": "admin", "password": "test-password",
	})
	var session struct {
		Token string `json:"token"`
	}
	decodeJSON(t, login, &session)
	updated := requestJSON(t, router, http.MethodPatch, "/api/settings/server.listen", session.Token,
		map[string]any{"value": "127.0.0.1:7100"})
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	value, err := store.GetSetting(ctx, "config.server.listen")
	if err != nil || value != "127.0.0.1:7100" {
		t.Fatalf("persisted value=%q err=%v", value, err)
	}
	invalid := requestJSON(t, router, http.MethodPatch, "/api/settings/server.listen", session.Token,
		map[string]any{"value": "not-an-address"})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid setting status=%d", invalid.Code)
	}
}

func TestSettingsPlainEnabledRequiresBooleanAndAddressPair(t *testing.T) {
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
	router := NewRouterWithOptions(store, service, time.Now(), RouterOptions{Settings: []SettingItem{
		{Key: "server.plain_enabled", Value: false, Mutable: true},
		{Key: "server.plain_listen", Value: "", Mutable: true},
		{Key: "server.plain_data_listen", Value: "127.0.0.1:7005", Mutable: true},
	}})
	session := loginForSettings(t, router)
	stringBool := requestJSON(t, router, http.MethodPatch, "/api/settings/server.plain_enabled", session,
		map[string]any{"value": "false"})
	if stringBool.Code != http.StatusBadRequest {
		t.Fatalf("string boolean status=%d", stringBool.Code)
	}
	missingPair := requestJSON(t, router, http.MethodPatch, "/api/settings/server.plain_enabled", session,
		map[string]any{"value": true})
	if missingPair.Code != http.StatusBadRequest {
		t.Fatalf("missing plaintext pair status=%d", missingPair.Code)
	}
	plainListen := requestJSON(t, router, http.MethodPatch, "/api/settings/server.plain_listen", session,
		map[string]any{"value": "127.0.0.1:7004"})
	if plainListen.Code != http.StatusOK {
		t.Fatalf("plain listen status=%d body=%s", plainListen.Code, plainListen.Body.String())
	}
	enabled := requestJSON(t, router, http.MethodPatch, "/api/settings/server.plain_enabled", session,
		map[string]any{"value": true})
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable status=%d body=%s", enabled.Code, enabled.Body.String())
	}
	value, err := store.GetSetting(ctx, "config.server.plain_enabled")
	if err != nil || value != "true" {
		t.Fatalf("persisted plain_enabled=%q err=%v", value, err)
	}
	disabled := requestJSON(t, router, http.MethodPatch, "/api/settings/server.plain_enabled", session,
		map[string]any{"value": false})
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	value, err = store.GetSetting(ctx, "config.server.plain_enabled")
	if err != nil || value != "false" {
		t.Fatalf("persisted disabled plain_enabled=%q err=%v", value, err)
	}
}

func loginForSettings(t *testing.T, router http.Handler) string {
	t.Helper()
	login := requestJSON(t, router, http.MethodPost, "/api/auth/login", "", map[string]any{
		"username": "admin", "password": "test-password",
	})
	var session struct {
		Token string `json:"token"`
	}
	decodeJSON(t, login, &session)
	return session.Token
}

func testRouter(t *testing.T) (http.Handler, func()) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.New(context.Background(), store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bootstrap(context.Background(), "admin", "test-password"); err != nil {
		t.Fatal(err)
	}
	return NewRouter(store, service, time.Now()), func() { _ = store.Close() }
}

func tokenRouterDependencies(t *testing.T) (*storage.Store, *auth.Service, string) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "pinned-token.db"))
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
	login := requestJSON(t, NewRouter(store, service, time.Now()), http.MethodPost,
		"/api/auth/login", "", map[string]any{"username": "admin", "password": "test-password"})
	var session struct {
		Token string `json:"token"`
	}
	decodeJSON(t, login, &session)
	return store, service, session.Token
}

func requestJSON(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &encoded)
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
