package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExampleConfigurationsUseSafeListenerDefaults(t *testing.T) {
	public := readExampleConfig(t, "config.example.yaml")
	if public.Server.Listen != "0.0.0.0:7000" || public.Server.DataListen != "0.0.0.0:7001" {
		t.Fatalf("public example does not listen on all interfaces: %+v", public.Server)
	}
	if public.Server.PlainEnabled {
		t.Fatalf("public example enables the legacy plaintext pair by default: %+v", public.Server)
	}
	if public.Server.TLS.Enabled {
		t.Fatal("public example must start with HTTP/WS until TLS is explicitly enabled")
	}
	if public.Server.AutoSubdomain.Enabled {
		t.Fatal("public example must leave automatic subdomain allocation disabled by default")
	}
	if public.Client.ServerURL != "ws://127.0.0.1:7000/agent/connect" {
		t.Fatalf("public example must use WS by default: %s", public.Client.ServerURL)
	}
	if err := ValidateServerTransport(public.Server); err != nil {
		t.Fatalf("public example violates transport policy: %v", err)
	}

	local := readExampleConfig(t, "config.local.example.yaml")
	if local.Server.AutoSubdomain.Enabled {
		t.Fatal("local example must leave automatic subdomain allocation disabled by default")
	}
	if err := ValidateServerTransport(local.Server); err != nil {
		t.Fatalf("local example violates transport policy: %v", err)
	}
}

func readExampleConfig(t *testing.T, name string) Config {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}
