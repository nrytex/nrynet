package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nat-link/nat-link/internal/config"
	"gopkg.in/yaml.v3"
)

type fileStore struct {
	path string
}

type diskConfig struct {
	Client            config.ClientConfig `yaml:"client"`
	UpdateManifestURL string              `yaml:"update_manifest_url"`
	UpdatePublicKey   string              `yaml:"update_public_key"`
	UpdateChannel     string              `yaml:"update_channel"`
	AutoStart         bool                `yaml:"auto_start"`
}

func newFileStore() (*fileStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "NAT-Link", "desktop.yaml")
	return &fileStore{path: path}, nil
}

func (s *fileStore) Load() (AppConfig, error) {
	var cfg diskConfig
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return AppConfig{}, nil
	}
	if err != nil {
		return AppConfig{}, fmt.Errorf("read desktop config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, fmt.Errorf("parse desktop config: %w", err)
	}
	out := configFromClient(cfg.Client)
	out.UpdateManifestURL = cfg.UpdateManifestURL
	out.UpdatePublicKey = cfg.UpdatePublicKey
	out.UpdateChannel = cfg.UpdateChannel
	out.AutoStart = cfg.AutoStart
	return out, nil
}

func (s *fileStore) Save(cfg AppConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	raw := diskConfig{
		Client: cfg.toClientConfig(), UpdateManifestURL: cfg.UpdateManifestURL,
		UpdatePublicKey: cfg.UpdatePublicKey, UpdateChannel: cfg.UpdateChannel,
		AutoStart: cfg.AutoStart,
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func (s *fileStore) Path() string {
	return s.path
}
