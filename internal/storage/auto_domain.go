package storage

import (
	"context"
	"strconv"
	"strings"
)

const (
	autoSubdomainEnabledKey = "config.server.auto_subdomain.enabled"
	autoSubdomainBaseKey    = "config.server.auto_subdomain.base_domain"
)

type AutoSubdomainConfig struct {
	Enabled    bool   `json:"enabled"`
	BaseDomain string `json:"base_domain"`
}

func (s *Store) SetAutoSubdomainConfig(ctx context.Context, cfg AutoSubdomainConfig) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	settings := []struct{ key, value string }{
		{autoSubdomainEnabledKey, strconv.FormatBool(cfg.Enabled)},
		{autoSubdomainBaseKey, cfg.BaseDomain},
	}
	for _, setting := range settings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?)
            ON CONFLICT(key) DO UPDATE SET value=excluded.value`, setting.key, setting.value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) AutoSubdomainConfig(ctx context.Context) (AutoSubdomainConfig, error) {
	var cfg AutoSubdomainConfig
	var enabled string
	err := s.db.QueryRowContext(ctx, `SELECT
        COALESCE(MAX(CASE WHEN key = ? THEN value END), ''),
        COALESCE(MAX(CASE WHEN key = ? THEN value END), '')
        FROM settings WHERE key IN (?, ?)`,
		autoSubdomainEnabledKey, autoSubdomainBaseKey,
		autoSubdomainEnabledKey, autoSubdomainBaseKey,
	).Scan(&enabled, &cfg.BaseDomain)
	if err != nil {
		return AutoSubdomainConfig{}, err
	}
	if enabled != "" {
		cfg.Enabled, err = strconv.ParseBool(enabled)
		if err != nil {
			return AutoSubdomainConfig{}, err
		}
	}
	return cfg, nil
}

func (s *Store) DomainInUse(ctx context.Context, protocol, domain string) (bool, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return false, err
	}
	var count int
	err = s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tunnels WHERE protocol = ? AND lower(domain) = lower(?)",
		strings.ToLower(protocol), domain).Scan(&count)
	return count > 0, err
}
