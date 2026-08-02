package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/endpoint"
)

const automaticUpdateInterval = 6 * time.Hour

type UpdateService struct {
	mu        sync.Mutex
	runner    updateRunner
	ready     bool
	configKey string
	checking  bool
}

type updateRunner interface {
	Init(updater.Config) error
	CheckAndInstall(context.Context) error
}

func NewUpdateService(runner updateRunner) *UpdateService {
	return &UpdateService{runner: runner}
}

func (s *UpdateService) ConfigureAutomatic(cfg AppConfig) error {
	if strings.TrimSpace(cfg.UpdateManifestURL) == "" && strings.TrimSpace(cfg.UpdatePublicKey) == "" {
		return nil
	}
	return s.ensureConfigured(cfg)
}

func (s *UpdateService) CheckAndInstall(cfg AppConfig) (UpdateResult, error) {
	if err := s.ensureConfigured(cfg); err != nil {
		return UpdateResult{}, err
	}
	s.mu.Lock()
	if s.checking {
		s.mu.Unlock()
		return UpdateResult{}, errors.New("an update check is already running")
	}
	s.checking = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.checking = false
		s.mu.Unlock()
	}()
	if err := s.runner.CheckAndInstall(context.Background()); err != nil {
		return UpdateResult{}, err
	}
	return UpdateResult{Started: true, Message: "update check completed"}, nil
}

func (s *UpdateService) ensureConfigured(cfg AppConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	manifestURL := strings.TrimSpace(cfg.UpdateManifestURL)
	if manifestURL == "" {
		return errors.New("update manifest URL is required")
	}
	provider, err := endpoint.New(endpoint.Config{URL: manifestURL, Channel: cfg.UpdateChannel})
	if err != nil {
		return err
	}
	publicKey, err := decodePublicKey(cfg.UpdatePublicKey)
	if err != nil {
		return err
	}
	configKey := strings.Join([]string{manifestURL, cfg.UpdateChannel, string(publicKey)}, "\x00")
	if s.ready {
		if s.configKey != configKey {
			return errors.New("update settings changed; restart NAT-Link before checking again")
		}
		return nil
	}
	err = s.runner.Init(updater.Config{
		CurrentVersion: appVersion,
		Providers:      []updater.Provider{provider},
		PublicKey:      publicKey,
		CheckInterval:  automaticUpdateInterval,
	})
	if err != nil && !errors.Is(err, updater.ErrAlreadyConfigured) {
		return err
	}
	s.ready = true
	s.configKey = configKey
	return nil
}

func decodePublicKey(raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, errors.New("update public key is required")
	}
	if strings.Contains(value, "BEGIN PUBLIC KEY") {
		return validatePublicKey([]byte(value))
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return validatePublicKey(data)
	}
	data, err = base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("update public key must be PEM or base64")
	}
	return validatePublicKey(data)
}

func validatePublicKey(raw []byte) ([]byte, error) {
	if len(raw) == ed25519.PublicKeySize {
		return raw, nil
	}
	block, _ := pem.Decode(raw)
	if block != nil {
		raw = block.Bytes
	}
	key, err := x509.ParsePKIXPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("update public key parse: %w", err)
	}
	if _, ok := key.(ed25519.PublicKey); !ok {
		return nil, fmt.Errorf("update public key must be Ed25519, got %T", key)
	}
	return raw, nil
}
