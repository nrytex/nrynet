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
	if !public.Server.TLS.Enabled {
		t.Fatal("public example must enable TLS for all-interface listeners")
	}
	if err := ValidateServerTransport(public.Server); err != nil {
		t.Fatalf("public example violates transport policy: %v", err)
	}

	local := readExampleConfig(t, "config.local.example.yaml")
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
