package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/storage"
	"github.com/nrytex/nrynet/server/api"
)

func safeSettings(cfg config.Config) []api.SettingItem {
	return []api.SettingItem{
		{Key: "server.p2p_enabled", Value: cfg.Server.P2PEnabled, Description: "启用 UDP 打洞和 TCP/P2P 直连，失败后自动使用 Relay；立即生效", Mutable: true},
		{Key: "server.plain_enabled", Value: cfg.Server.PlainEnabled, Description: "是否启用额外的兼容 HTTP、WS 和明文 TCP 监听；热更新", Mutable: true},
		{Key: "server.listen", Value: cfg.Server.Listen, Description: "主 HTTP/WS 与 HTTPS/WSS 同端口监听地址；端口修改需重启", Mutable: true},
		{Key: "server.plain_listen", Value: cfg.Server.PlainListen, Description: "额外兼容 HTTP/WS 地址；开启兼容访问时与数据地址同时生效", Mutable: true},
		{Key: "server.data_listen", Value: cfg.Server.DataListen, Description: "主明文与 TLS 数据同端口监听地址；端口修改需重启", Mutable: true},
		{Key: "server.plain_data_listen", Value: cfg.Server.PlainDataListen, Description: "额外兼容明文数据地址；开启兼容访问时与控制地址同时生效", Mutable: true},
		{Key: "server.public_data_address", Value: cfg.Server.PublicDataAddress, Description: "提供给 Agent 和 Relay 的数据通道地址；重启后生效", Mutable: true},
		{Key: "server.quic_listen", Value: cfg.Server.QUICListen, Description: "QUIC 控制与数据流监听地址；重启后生效", Mutable: true},
		{Key: "server.public_quic_address", Value: cfg.Server.PublicQUICAddress, Description: "提供给 Agent 的 QUIC 地址；重启后生效", Mutable: true},
		{Key: "server.rendezvous_listen", Value: cfg.Server.RendezvousListen, Description: "UDP Rendezvous 监听地址；重启后生效", Mutable: true},
		{Key: "server.public_rendezvous_address", Value: cfg.Server.PublicRendezvous, Description: "提供给 Agent 的 Rendezvous 地址；重启后生效", Mutable: true},
		{Key: "server.http_listen", Value: cfg.Server.HTTPListen, Description: "HTTP Host 与 HTTPS SNI 网关地址；重启后生效", Mutable: true},
		{Key: "server.tls.enabled", Value: cfg.Server.TLS.Enabled, Description: "启用 HTTPS、WSS 与 TLS 数据通道；热更新", Mutable: true},
		{Key: "server.tls.cert_file", Value: cfg.Server.TLS.CertFile, Description: "TLS 证书链文件路径；证书管理器热加载", Mutable: true},
		{Key: "server.tls.key_file", Value: cfg.Server.TLS.KeyFile, Description: "TLS 私钥文件路径；证书管理器热加载", Mutable: true},
		{Key: "server.heartbeat_timeout", Value: cfg.Server.HeartbeatText, Description: "Agent 离线判定超时时间；重启后生效", Mutable: true},
	}
}

func applyStoredSettings(ctx context.Context, store *storage.Store, cfg *config.Config) error {
	stringSettings := []struct {
		key    string
		target *string
	}{
		{"server.listen", &cfg.Server.Listen},
		{"server.plain_listen", &cfg.Server.PlainListen},
		{"server.data_listen", &cfg.Server.DataListen},
		{"server.plain_data_listen", &cfg.Server.PlainDataListen},
		{"server.public_data_address", &cfg.Server.PublicDataAddress},
		{"server.quic_listen", &cfg.Server.QUICListen},
		{"server.public_quic_address", &cfg.Server.PublicQUICAddress},
		{"server.rendezvous_listen", &cfg.Server.RendezvousListen},
		{"server.public_rendezvous_address", &cfg.Server.PublicRendezvous},
		{"server.http_listen", &cfg.Server.HTTPListen},
		{"server.tls.cert_file", &cfg.Server.TLS.CertFile},
		{"server.tls.key_file", &cfg.Server.TLS.KeyFile},
		{"server.tls.domain", &cfg.Server.TLS.Domain},
		{"server.tls.email", &cfg.Server.TLS.Email},
		{"server.auto_subdomain.base_domain", &cfg.Server.AutoSubdomain.BaseDomain},
		{"server.heartbeat_timeout", &cfg.Server.HeartbeatText},
	}
	for _, setting := range stringSettings {
		value, err := store.GetSetting(ctx, "config."+setting.key)
		if err == nil {
			*setting.target = value
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if cfg.Server.AutoSubdomain.BaseDomain != "" {
		normalized, err := storage.NormalizeDomain(cfg.Server.AutoSubdomain.BaseDomain)
		if err != nil {
			return err
		}
		cfg.Server.AutoSubdomain.BaseDomain = normalized
	}
	duration, err := time.ParseDuration(cfg.Server.HeartbeatText)
	if err != nil {
		return fmt.Errorf("stored heartbeat timeout: %w", err)
	}
	cfg.Server.HeartbeatTimeout = duration
	if value, err := store.GetSetting(ctx, "config.server.plain_enabled"); err == nil {
		cfg.Server.PlainEnabled, err = strconv.ParseBool(value)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if value, err := store.GetSetting(ctx, "config.server.p2p_enabled"); err == nil {
		cfg.Server.P2PEnabled, err = strconv.ParseBool(value)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if value, err := store.GetSetting(ctx, "config.server.tls.enabled"); err == nil {
		cfg.Server.TLS.Enabled, err = strconv.ParseBool(value)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if value, err := store.GetSetting(ctx, "config.server.auto_subdomain.enabled"); err == nil {
		cfg.Server.AutoSubdomain.Enabled, err = strconv.ParseBool(value)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if cfg.Server.AutoSubdomain.Enabled && cfg.Server.AutoSubdomain.BaseDomain == "" {
		return errors.New("server.auto_subdomain.base_domain is required")
	}
	return seedAutoSubdomainSettings(ctx, store, cfg.Server.AutoSubdomain)
}

func seedAutoSubdomainSettings(ctx context.Context, store *storage.Store, cfg config.AutoSubdomainConfig) error {
	return store.SetAutoSubdomainConfig(ctx, storage.AutoSubdomainConfig{
		Enabled: cfg.Enabled, BaseDomain: cfg.BaseDomain,
	})
}
