package app

import (
	"context"
	"errors"
	"strings"

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/storage"
	"github.com/nrytex/nrynet/server/api"
)

func (c *TransportController) SetAutoSubdomain(ctx context.Context, request api.AutoSubdomainRequest) (api.TransportStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.app == nil {
		return c.statusLocked(), errors.New("transport manager not available")
	}
	if request.Enabled == nil {
		return c.statusLocked(), errors.New("enabled is required")
	}
	baseDomain, err := normalizeAutoSubdomainBase(request.BaseDomain)
	if err != nil {
		return c.statusLocked(), err
	}
	enabled := *request.Enabled
	if enabled && baseDomain == "" {
		return c.statusLocked(), errors.New("server.auto_subdomain.base_domain is required")
	}
	next := config.AutoSubdomainConfig{Enabled: enabled, BaseDomain: baseDomain}
	stored := storage.AutoSubdomainConfig{Enabled: next.Enabled, BaseDomain: next.BaseDomain}
	if err := c.app.store.SetAutoSubdomainConfig(ctx, stored); err != nil {
		return c.statusLocked(), err
	}
	c.app.config.Server.AutoSubdomain = next
	return c.statusLocked(), nil
}

func normalizeAutoSubdomainBase(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	return storage.NormalizeDomain(value)
}

func autoSubdomainStatus(cfg config.AutoSubdomainConfig) api.TransportAutoSubdomain {
	status := api.TransportAutoSubdomain{Enabled: cfg.Enabled, BaseDomain: strings.TrimSpace(cfg.BaseDomain)}
	if status.BaseDomain != "" {
		status.SuffixExample = "app." + status.BaseDomain
	}
	return status
}
