package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/storage"
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
