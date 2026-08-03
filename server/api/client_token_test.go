package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/nrytex/nrynet/internal/storage"
)

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
	var resetToken struct {
		Value string `json:"value"`
	}
	decodeJSON(t, reset, &resetToken)
	if !strings.Contains(resetToken.Value, ".spki-sha256-") {
		t.Fatalf("reset token does not include certificate pin: %q", resetToken.Value)
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
