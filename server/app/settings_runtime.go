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
		{Key: "server.plain_enabled", Value: cfg.Server.PlainEnabled, Description: "是否启用明文 HTTP、WS 和 TCP 监听；重启后生效", Mutable: true},
		{Key: "server.listen", Value: cfg.Server.Listen, Description: "HTTPS 控制台与 WSS 控制通道地址；重启后生效", Mutable: true},
		{Key: "server.plain_listen", Value: cfg.Server.PlainListen, Description: "可选 HTTP 控制台与 WS 控制通道地址；开启明文访问时需与明文数据通道同时填写；重启后生效", Mutable: true},
		{Key: "server.data_listen", Value: cfg.Server.DataListen, Description: "TLS TCP 数据通道地址；重启后生效", Mutable: true},
		{Key: "server.plain_data_listen", Value: cfg.Server.PlainDataListen, Description: "可选明文 TCP 数据通道地址；开启明文访问时需与明文控制通道同时填写；重启后生效", Mutable: true},
		{Key: "server.public_data_address", Value: cfg.Server.PublicDataAddress, Description: "提供给 Agent 和 Relay 的数据通道地址；重启后生效", Mutable: true},
		{Key: "server.quic_listen", Value: cfg.Server.QUICListen, Description: "QUIC 控制与数据流监听地址；重启后生效", Mutable: true},
		{Key: "server.public_quic_address", Value: cfg.Server.PublicQUICAddress, Description: "提供给 Agent 的 QUIC 地址；重启后生效", Mutable: true},
		{Key: "server.rendezvous_listen", Value: cfg.Server.RendezvousListen, Description: "UDP Rendezvous 监听地址；重启后生效", Mutable: true},
		{Key: "server.public_rendezvous_address", Value: cfg.Server.PublicRendezvous, Description: "提供给 Agent 的 Rendezvous 地址；重启后生效", Mutable: true},
		{Key: "server.http_listen", Value: cfg.Server.HTTPListen, Description: "HTTP Host 与 HTTPS SNI 网关地址；重启后生效", Mutable: true},
		{Key: "server.tls.enabled", Value: cfg.Server.TLS.Enabled, Description: "启用 HTTPS、WSS、TLS 数据通道和证书指纹；重启后生效", Mutable: true},
		{Key: "server.tls.cert_file", Value: cfg.Server.TLS.CertFile, Description: "TLS 证书链文件路径；重启后生效", Mutable: true},
		{Key: "server.tls.key_file", Value: cfg.Server.TLS.KeyFile, Description: "TLS 私钥文件路径；重启后生效", Mutable: true},
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
	if value, err := store.GetSetting(ctx, "config.server.tls.enabled"); err == nil {
		cfg.Server.TLS.Enabled, err = strconv.ParseBool(value)
		return err
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}
