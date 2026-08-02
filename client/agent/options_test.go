package agent

import (
	"testing"

	"github.com/nat-link/nat-link/internal/config"
)

func TestNewOptionsNormalizesClientIdentity(t *testing.T) {
	options := NewOptions(config.Config{Client: config.ClientConfig{
		ServerURL:   "ws://127.0.0.1/agent/connect",
		DataAddress: "127.0.0.1:7001",
		Token:       "token",
	}}, "test")
	if options.Config.Name == "" {
		t.Fatal("expected default name")
	}
	if options.Config.DeviceID == "" {
		t.Fatal("expected default device id")
	}
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
}
