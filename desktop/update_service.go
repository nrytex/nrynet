package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

const (
	githubRepository       = "nrytex/nrynet"
	automaticCheckInterval = 6 * time.Hour
)

type UpdateService struct {
	mu       sync.Mutex
	runner   updateRunner
	ready    bool
	checking bool
	last     UpdateResult
	stop     chan struct{}
}

type updateRunner interface {
	Init(updater.Config) error
	Check(context.Context) (*updater.Release, error)
}

func NewUpdateService(runner updateRunner) *UpdateService {
	return &UpdateService{runner: runner}
}

func (s *UpdateService) ConfigureAutomatic() error {
	return s.ensureConfigured()
}

func (s *UpdateService) StartAutomaticChecks(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		return
	}
	stop := make(chan struct{})
	s.stop = stop
	go s.checkLoop(stop, interval)
}

func (s *UpdateService) StopAutomaticChecks() {
	s.mu.Lock()
	stop := s.stop
	s.stop = nil
	s.mu.Unlock()
	if stop != nil {
		close(stop)
	}
}

func (s *UpdateService) checkLoop(stop chan struct{}, interval time.Duration) {
	_, _ = s.checkOnce()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			_, _ = s.checkOnce()
		}
	}
}

func (s *UpdateService) CheckForUpdate() (UpdateResult, error) {
	if err := s.ensureConfigured(); err != nil {
		return UpdateResult{}, err
	}
	return s.checkOnce()
}

func (s *UpdateService) checkOnce() (UpdateResult, error) {
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
	release, err := s.runner.Check(context.Background())
	if err != nil {
		return UpdateResult{}, err
	}
	result := updateResultFromRelease(release)
	s.mu.Lock()
	s.last = result
	s.mu.Unlock()
	return result, nil
}

// LastResult returns the latest check result, or nil when no new version is
// available. Frontend reads it through DesktopSnapshot to show a subtle
// banner without any system popup.
func (s *UpdateService) LastResult() *UpdateResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.last.Available {
		return nil
	}
	result := s.last
	return &result
}

func updateResultFromRelease(release *updater.Release) UpdateResult {
	if release == nil {
		return UpdateResult{Checked: true, Message: "当前已是最新版本。"}
	}
	downloadURL, _ := releaseDownloadURL(release)
	return UpdateResult{
		Checked:       true,
		Available:     true,
		LatestVersion: release.Version,
		DownloadURL:   downloadURL,
		Message:       "发现新版本 " + release.Version + "，请前往 GitHub Release 下载。",
	}
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
		Window:         updater.WindowNone,
	})
	if err != nil && !errors.Is(err, updater.ErrAlreadyConfigured) {
		return err
	}
	s.ready = true
	return nil
}
