package agent

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/nat-link/nat-link/internal/config"
)

const defaultHeartbeatInterval = 15 * time.Second

type Options struct {
	Config            config.ClientConfig
	Version           string
	HeartbeatInterval time.Duration
	ReconnectMin      time.Duration
	ReconnectMax      time.Duration
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
	if o.Config.DataAddress == "" {
		return errors.New("client.data_address is required")
	}
	if o.Config.Token == "" {
		return errors.New("client.token is required")
	}
	if o.Config.DeviceID == "" {
		return errors.New("client.device_id is required")
	}
	return nil
}

func normalizeClientConfig(cfg config.ClientConfig) config.ClientConfig {
	hostname, _ := os.Hostname()
	if cfg.Name == "" {
		cfg.Name = hostname
	}
	if cfg.DeviceID == "" {
		cfg.DeviceID = fmt.Sprintf("%s-%s-%s", runtime.GOOS, runtime.GOARCH, hostname)
	}
	return cfg
}
