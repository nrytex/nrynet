package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

const (
	automaticUpdateInterval = 6 * time.Hour
	githubRepository        = "nrytex/nrynet"
)

type UpdateService struct {
	mu       sync.Mutex
	runner   updateRunner
	ready    bool
	checking bool
}

type updateRunner interface {
	Init(updater.Config) error
	CheckAndInstall(context.Context) error
}

func NewUpdateService(runner updateRunner) *UpdateService {
	return &UpdateService{runner: runner}
}

func (s *UpdateService) ConfigureAutomatic() error {
	return s.ensureConfigured()
}

func (s *UpdateService) CheckAndInstall() (UpdateResult, error) {
	if err := s.ensureConfigured(); err != nil {
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
	return UpdateResult{Started: true, Message: "已完成 GitHub 版本检查。"}, nil
}

func (s *UpdateService) ensureConfigured() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ready {
		return nil
	}
	provider, err := newGitHubReleaseProvider(githubRepository, nil)
	if err != nil {
		return err
	}
	err = s.runner.Init(updater.Config{
		CurrentVersion: appVersion,
		Providers:      []updater.Provider{provider},
		CheckInterval:  automaticUpdateInterval,
	})
	if err != nil && !errors.Is(err, updater.ErrAlreadyConfigured) {
		return err
	}
	s.ready = true
	return nil
}
