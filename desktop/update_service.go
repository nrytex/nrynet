package main

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

const (
	githubRepository       = "nrytex/nrynet"
	automaticCheckInterval = 6 * time.Hour
)

type UpdateService struct {
	mu                sync.Mutex
	flowMu            sync.Mutex
	runner            updateRunner
	ready             bool
	checking          bool
	last              UpdateResult
	downloadedVersion string
	downloadStarted   bool
	stop              chan struct{}
}

type updateRunner interface {
	Init(updater.Config) error
	Check(context.Context) (*updater.Release, error)
	DownloadAndInstall(context.Context) error
	Restart(context.Context) error
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
	_, _ = s.runCheckAndDownload()
	if interval <= 0 {
		interval = automaticCheckInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			_, _ = s.runCheckAndDownload()
		}
	}
}

func (s *UpdateService) CheckForUpdate() (UpdateResult, error) {
	if err := s.ensureConfigured(); err != nil {
		return UpdateResult{}, err
	}
	return s.runCheckAndDownload()
}

func (s *UpdateService) ApplyUpdate() error {
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	s.flowMu.Lock()
	defer s.flowMu.Unlock()
	return s.runner.Restart(context.Background())
}

func (s *UpdateService) runCheckAndDownload() (UpdateResult, error) {
	s.flowMu.Lock()
	defer s.flowMu.Unlock()
	result, err := s.checkOnce()
	if err != nil || !result.Available {
		return result, err
	}
	return s.download(result)
}

func (s *UpdateService) download(result UpdateResult) (UpdateResult, error) {
	s.mu.Lock()
	if s.downloadedVersion == result.LatestVersion {
		result.DownloadState = "ready"
		result.Ready = true
		result.Message = "新版本 " + result.LatestVersion + " 已下载，重启应用即可完成更新。"
		s.last = result
		s.mu.Unlock()
		return result, nil
	}
	if s.downloadStarted {
		result.DownloadState = "downloading"
		result.Message = "正在下载更新版本 " + result.LatestVersion + "..."
		s.last = result
		s.mu.Unlock()
		return result, nil
	}
	s.downloadStarted = true
	result.DownloadState = "downloading"
	result.Message = "正在下载更新版本 " + result.LatestVersion + "..."
	s.last = result
	s.mu.Unlock()

	if err := s.runner.DownloadAndInstall(context.Background()); err != nil {
		result.DownloadState = "error"
		result.Message = "更新下载失败：" + err.Error()
		s.mu.Lock()
		s.downloadStarted = false
		s.last = result
		s.mu.Unlock()
		return result, err
	}

	result.DownloadState = "ready"
	result.Ready = true
	result.Message = "新版本 " + result.LatestVersion + " 已下载，重启应用即可完成更新。"
	s.mu.Lock()
	s.downloadStarted = false
	s.downloadedVersion = result.LatestVersion
	s.last = result
	s.mu.Unlock()
	return result, nil
}

func (s *UpdateService) checkOnce() (result UpdateResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("update check panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
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
	result = updateResultFromRelease(release)
	if release == nil {
		s.mu.Lock()
		s.last = result
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Lock()
	if s.downloadedVersion == result.LatestVersion {
		result.DownloadState = "ready"
		result.Ready = true
		result.Message = "新版本 " + result.LatestVersion + " 已下载，重启应用即可完成更新。"
	}
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
	if !s.last.Available || s.last.DownloadState == "error" {
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
		Message:       "发现新版本 " + release.Version,
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
