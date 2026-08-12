package agent

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/nrytex/nrynet/internal/config"
)

const defaultHeartbeatInterval = 10 * time.Second

type Options struct {
	Config            config.ClientConfig
	Version           string
	HeartbeatInterval time.Duration
	ReconnectMin      time.Duration
	ReconnectMax      time.Duration
	Observer          Observer
}

func NewOptions(cfg config.Config, version string) Options {
	return Options{
		Config:            normalizeClientConfig(cfg.Client),
		Version:           version,
		HeartbeatInterval: defaultHeartbeatInterval,
		ReconnectMin:      time.Second,
		ReconnectMax:      30 * time.Second,
	}
}

func (o Options) Validate() error {
	if o.Config.ServerURL == "" {
		return errors.New("client.server_url is required")
	}
	if o.Config.Transport == "quic" && o.Config.QUICAddress == "" {
		return errors.New("client.quic_address is required")
	}
	if o.Config.Transport != "quic" && o.Config.Transport != "websocket" {
		return errors.New("client.transport must be websocket or quic")
	}
	if o.Config.Transport != "quic" && o.Config.DataAddress == "" {
		return errors.New("client.data_address is required")
	}
	if o.Config.Transport == "websocket" {
		if err := config.ValidateSecureWebSocketURL(o.Config.ServerURL, o.Config.DataAddress); err != nil {
			return err
		}
	}
	if err := config.ValidateTLSVerification(o.Config); err != nil {
		return err
	}
	if o.Config.Token == "" {
		return errors.New("client.token is required")
	}
	if o.Config.DeviceID == "" {
		return errors.New("client.device_id is required")
	}
	return nil
}

func normalizeOptions(options Options) Options {
	if options.HeartbeatInterval <= 0 {
		options.HeartbeatInterval = defaultHeartbeatInterval
	}
	if options.ReconnectMin <= 0 {
		options.ReconnectMin = time.Second
	}
	if options.ReconnectMax < options.ReconnectMin {
		options.ReconnectMax = 30 * time.Second
		if options.ReconnectMax < options.ReconnectMin {
			options.ReconnectMax = options.ReconnectMin
		}
	}
	return options
}

func normalizeClientConfig(cfg config.ClientConfig) config.ClientConfig {
	cfg.Transport = strings.ToLower(cfg.Transport)
	if cfg.Transport == "" {
		cfg.Transport = "websocket"
	}
	hostname, _ := os.Hostname()
	if cfg.Name == "" {
		cfg.Name = hostname
	}
	if cfg.DeviceID == "" {
		cfg.DeviceID = fmt.Sprintf("%s-%s-%s", runtime.GOOS, runtime.GOARCH, hostname)
	}
	return cfg
}
