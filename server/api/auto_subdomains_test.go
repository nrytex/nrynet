package api

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/storage"
)

func TestCreateTunnelAssignsAutoSubdomainWhenDomainIsEmpty(t *testing.T) {
	store, service, router, session, _ := managementRouter(t)
	client := autoSubdomainClient(t, store, service)
	enableAutoSubdomain(t, store, "example.com")
	created := createAutoSubdomainTunnel(t, router, session, client.ID, "服务", "http")
	if created.Domain != "tunnel.example.com" {
		t.Fatalf("domain=%q", created.Domain)
	}
	second := createAutoSubdomainTunnel(t, router, session, client.ID, "服务", "http")
	if second.Domain != "tunnel-2.example.com" {
		t.Fatalf("second domain=%q", second.Domain)
	}
	secure := createAutoSubdomainTunnel(t, router, session, client.ID, "服务", "https")
	if secure.Domain != "tunnel.example.com" {
		t.Fatalf("https domain=%q", secure.Domain)
	}
}

func TestCreateTunnelKeepsExplicitDomainAndDisabledBehavior(t *testing.T) {
	store, service, router, session, _ := managementRouter(t)
	client := autoSubdomainClient(t, store, service)
	enableAutoSubdomain(t, store, "example.com")
	explicit := requestJSON(t, router, http.MethodPost, "/api/tunnels", session, map[string]any{
		"name": "explicit", "client_id": client.ID, "protocol": "http",
		"local_host": "127.0.0.1", "local_port": 8080, "domain": "App.Example.NET.",
	})
	if explicit.Code != http.StatusCreated {
		t.Fatalf("explicit status=%d body=%s", explicit.Code, explicit.Body.String())
	}
	var created model.Tunnel
	decodeJSON(t, explicit, &created)
	if created.Domain != "app.example.net" {
		t.Fatalf("explicit domain=%q", created.Domain)
	}
	if err := store.SetSetting(context.Background(), "config.server.auto_subdomain.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	missing := requestJSON(t, router, http.MethodPost, "/api/tunnels", session, map[string]any{
		"name": "missing", "client_id": client.ID, "protocol": "http",
		"local_host": "127.0.0.1", "local_port": 8080,
	})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("disabled missing-domain status=%d", missing.Code)
	}
}

func TestAutoSubdomainConcurrentCreateRetriesConflicts(t *testing.T) {
	store, service, router, session, _ := managementRouter(t)
	client := autoSubdomainClient(t, store, service)
	enableAutoSubdomain(t, store, "example.com")
	const count = 6
	var wg sync.WaitGroup
	domains := make(chan string, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tunnel := createAutoSubdomainTunnel(t, router, session, client.ID, "same", "http")
			domains <- tunnel.Domain
		}()
	}
	wg.Wait()
	close(domains)
	seen := make(map[string]bool, count)
	for domain := range domains {
		if seen[domain] {
			t.Fatalf("duplicate domain allocated: %s", domain)
		}
		seen[domain] = true
	}
	for _, domain := range []string{"same.example.com", "same-2.example.com", "same-6.example.com"} {
		if !seen[domain] {
			t.Fatalf("expected %s in allocated domains: %v", domain, seen)
		}
	}
}

func TestAutoSubdomainAllocatesBeyondTwentyDuplicateNames(t *testing.T) {
	store, service, router, session, _ := managementRouter(t)
	client := autoSubdomainClient(t, store, service)
	enableAutoSubdomain(t, store, "example.com")
	var created model.Tunnel
	for i := 0; i < 21; i++ {
		created = createAutoSubdomainTunnel(t, router, session, client.ID, "same", "http")
	}
	if created.Domain != "same-21.example.com" {
		t.Fatalf("domain=%q", created.Domain)
	}
}

func autoSubdomainClient(t *testing.T, store *storage.Store, service interface {
	CreateAgentToken(context.Context, string) (model.Token, string, error)
}) model.Client {
	t.Helper()
	token, _, err := service.CreateAgentToken(context.Background(), "auto-subdomain")
	if err != nil {
		t.Fatal(err)
	}
	client, err := store.UpsertClient(context.Background(), token.ID, storage.ClientHello{
		Name: "auto", DeviceID: "auto-device",
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func enableAutoSubdomain(t *testing.T, store *storage.Store, baseDomain string) {
	t.Helper()
	if err := store.SetSetting(context.Background(), "config.server.auto_subdomain.enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(context.Background(), "config.server.auto_subdomain.base_domain", baseDomain); err != nil {
		t.Fatal(err)
	}
}

func createAutoSubdomainTunnel(
	t *testing.T,
	router http.Handler,
	session,
	clientID,
	name,
	protocol string,
) model.Tunnel {
	t.Helper()
	response := requestJSON(t, router, http.MethodPost, "/api/tunnels", session, map[string]any{
		"name": name, "client_id": clientID, "protocol": protocol,
		"local_host": "127.0.0.1", "local_port": 8080,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var tunnel model.Tunnel
	decodeJSON(t, response, &tunnel)
	return tunnel
}
