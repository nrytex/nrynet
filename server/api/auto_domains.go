package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/storage"
)

func (h tunnelHandler) createTunnel(ctx context.Context, tunnel model.Tunnel) (model.Tunnel, error) {
	if !needsAutoDomain(tunnel) {
		return h.store.CreateTunnel(ctx, normalizeTunnelDomain(tunnel))
	}
	cfg, err := h.store.AutoSubdomainConfig(ctx)
	if err != nil {
		return model.Tunnel{}, fmt.Errorf("load auto-subdomain settings: %w", err)
	}
	if !cfg.Enabled {
		return h.store.CreateTunnel(ctx, tunnel)
	}
	baseDomain, err := storage.NormalizeDomain(cfg.BaseDomain)
	if err != nil {
		return model.Tunnel{}, errors.New("auto-subdomain base domain is invalid")
	}
	slug, err := storage.SlugFromName(tunnel.Name)
	if err != nil {
		return model.Tunnel{}, errors.New("tunnel name cannot be converted to a safe subdomain")
	}
	protocol := strings.ToLower(tunnel.Protocol)
	for attempt := 0; ; attempt++ {
		tunnel.Domain, err = candidateDomain(slug, baseDomain, attempt)
		if err != nil {
			return model.Tunnel{}, err
		}
		created, err := h.store.CreateTunnel(ctx, tunnel)
		if err == nil {
			return created, nil
		}
		if !isDomainConflict(err) {
			return model.Tunnel{}, err
		}
		if inUse, lookupErr := h.store.DomainInUse(ctx, protocol, tunnel.Domain); lookupErr != nil || !inUse {
			return model.Tunnel{}, err
		}
	}
}

func needsAutoDomain(tunnel model.Tunnel) bool {
	protocol := strings.ToLower(tunnel.Protocol)
	return (protocol == "http" || protocol == "https") && strings.TrimSpace(tunnel.Domain) == ""
}

func normalizeTunnelDomain(tunnel model.Tunnel) model.Tunnel {
	protocol := strings.ToLower(tunnel.Protocol)
	if protocol != "http" && protocol != "https" || strings.TrimSpace(tunnel.Domain) == "" {
		return tunnel
	}
	tunnel.Domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(tunnel.Domain)), ".")
	return tunnel
}

func candidateDomain(slug, baseDomain string, attempt int) (string, error) {
	candidateSlug := slug
	if attempt > 0 {
		candidateSlug = fmt.Sprintf("%s-%d", trimSlugForSuffix(slug, attempt), attempt+1)
	}
	return storage.JoinDomain(candidateSlug, baseDomain)
}

func trimSlugForSuffix(slug string, attempt int) string {
	suffixLen := len(fmt.Sprintf("-%d", attempt+1))
	limit := 63 - suffixLen
	if len(slug) <= limit {
		return slug
	}
	return strings.TrimRight(slug[:limit], "-")
}

func isDomainConflict(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "idx_tunnels_protocol_domain") ||
		strings.Contains(message, "unique") && strings.Contains(message, "domain")
}
