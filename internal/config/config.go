package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type BootstrapConfig struct {
	AdminUsername string `yaml:"admin_username"`
	AdminPassword string `yaml:"admin_password"`
}

type ServerConfig struct {
	Listen            string          `yaml:"listen"`
	DataListen        string          `yaml:"data_listen"`
	PublicDataAddress string          `yaml:"public_data_address"`
	QUICListen        string          `yaml:"quic_listen"`
	PublicQUICAddress string          `yaml:"public_quic_address"`
	RendezvousListen  string          `yaml:"rendezvous_listen"`
	PublicRendezvous  string          `yaml:"public_rendezvous_address"`
	RelayAPIToken     string          `yaml:"relay_api_token"`
	HTTPListen        string          `yaml:"http_listen"`
	Database          string          `yaml:"database"`
	LogDirectory      string          `yaml:"log_directory"`
	JWTTTL            time.Duration   `yaml:"-"`
	JWTTTLText        string          `yaml:"jwt_ttl"`
	HeartbeatTimeout  time.Duration   `yaml:"-"`
	HeartbeatText     string          `yaml:"heartbeat_timeout"`
	TLS               TLSConfig       `yaml:"tls"`
	Bootstrap         BootstrapConfig `yaml:"bootstrap"`
}

type ClientConfig struct {
	ServerURL          string `yaml:"server_url"`
	DataAddress        string `yaml:"data_address"`
	Transport          string `yaml:"transport"`
	QUICAddress        string `yaml:"quic_address"`
	RendezvousAddress  string `yaml:"rendezvous_address"`
	CAFile             string `yaml:"ca_file"`
	Token              string `yaml:"token"`
	Name               string `yaml:"name"`
	DeviceID           string `yaml:"device_id"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type Config struct {
	Server ServerConfig `yaml:"server"`
	Client ClientConfig `yaml:"client"`
}

func Load(path string) (Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config: %w", err)
		}
	}
	if err := cfg.parseDurations(); err != nil {
		return Config{}, err
	}
	if err := ensureParent(cfg.Server.Database); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaults() Config {
	return Config{Server: ServerConfig{
		Listen: "127.0.0.1:7000", DataListen: "127.0.0.1:7001",
		PublicDataAddress: "127.0.0.1:7001", HTTPListen: "127.0.0.1:8080",
		QUICListen: "127.0.0.1:7002", PublicQUICAddress: "127.0.0.1:7002",
		RendezvousListen: "127.0.0.1:7003", PublicRendezvous: "127.0.0.1:7003",
		Database: "data/nrynet.db", LogDirectory: "logs",
		JWTTTLText: "12h", HeartbeatText: "45s",
		Bootstrap: BootstrapConfig{AdminUsername: "admin"},
	}}
}

func (c *Config) parseDurations() error {
	var err error
	c.Server.JWTTTL, err = time.ParseDuration(c.Server.JWTTTLText)
	if err != nil {
		return fmt.Errorf("invalid server.jwt_ttl: %w", err)
	}
	c.Server.HeartbeatTimeout, err = time.ParseDuration(c.Server.HeartbeatText)
	if err != nil {
		return fmt.Errorf("invalid server.heartbeat_timeout: %w", err)
	}
	return nil
}

func ensureParent(path string) error {
	dir := filepath.Dir(path)
	if dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o750)
}

func RandomSecret(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
