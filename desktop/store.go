package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nrytex/nrynet/internal/config"
	"gopkg.in/yaml.v3"
)

type fileStore struct {
	path       string
	legacyPath string
}

type diskConfig struct {
	Client    config.ClientConfig `yaml:"client"`
	AutoStart bool                `yaml:"auto_start"`
}

func newFileStore() (*fileStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &fileStore{
		path:       filepath.Join(dir, "Nrynet", "desktop.yaml"),
		legacyPath: filepath.Join(dir, "NAT-Link", "desktop.yaml"),
	}, nil
}

func (s *fileStore) Load() (AppConfig, error) {
	var cfg diskConfig
	data, err := os.ReadFile(s.loadPath())
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
	out.AutoStart = cfg.AutoStart
	return out, nil
}

func (s *fileStore) loadPath() string {
	_, err := os.Stat(s.path)
	if err == nil || !errors.Is(err, os.ErrNotExist) || s.legacyPath == "" {
		return s.path
	}
	return s.legacyPath
}

func (s *fileStore) Save(cfg AppConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	raw := diskConfig{
		Client: cfg.toClientConfig(), AutoStart: cfg.AutoStart,
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
