package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nat-link/nat-link/internal/storage"
)

func TestBootstrapLoginAndAgentToken(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err := New(ctx, store, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Bootstrap(ctx, "admin", "test-password")
	if err != nil || !result.Created {
		t.Fatalf("bootstrap: result=%+v err=%v", result, err)
	}
	session, err := service.Login(ctx, "admin", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.VerifyJWT(session); err != nil {
		t.Fatal(err)
	}
	token, cleartext, err := service.CreateAgentToken(ctx, "home")
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := service.AuthenticateAgent(ctx, cleartext)
	if err != nil || authenticated.ID != token.ID {
		t.Fatalf("authenticate token: token=%+v err=%v", authenticated, err)
	}
}
