package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/model"
)

type DesktopService struct {
	mu          sync.Mutex
	store       *fileStore
	cfg         AppConfig
	status      RuntimeStatus
	logs        *memoryLogHandler
	cancel      context.CancelFunc
	runID       uint64
	updater     *UpdateService
	tunnels     []model.Tunnel
	tunnelPaths map[string]string
	window      windowControls
	browser     browserOpener
}

type browserOpener interface {
	OpenURL(string) error
}

type windowControls interface {
	Show()
	Hide()
	Quit()
}

func NewDesktopService(store *fileStore, logs *memoryLogHandler, updater *UpdateService) (*DesktopService, error) {
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}
	if err := updater.ConfigureAutomatic(); err != nil {
		return nil, fmt.Errorf("configure automatic updates: %w", err)
	}
	svc := &DesktopService{
		store: store, cfg: cfg, logs: logs, updater: updater,
		status:      RuntimeStatus{State: "disconnected", Version: appVersion},
		tunnels:     make([]model.Tunnel, 0),
		tunnelPaths: make(map[string]string),
	}
	updater.StartAutomaticChecks(automaticCheckInterval)
	return svc, nil
}

func (s *DesktopService) setWindow(window windowControls) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.window = window
}

func (s *DesktopService) setBrowser(browser browserOpener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.browser = browser
}

func (s *DesktopService) Snapshot() DesktopSnapshot {
	s.mu.Lock()
	cfg, status := s.cfg, s.status
	tunnels := append([]model.Tunnel{}, s.tunnels...)
	tunnelPaths := copyTunnelPaths(s.tunnelPaths)
	var update *UpdateResult
	if s.updater != nil {
		update = s.updater.LastResult()
	}
	s.mu.Unlock()
	return DesktopSnapshot{Config: cfg, Status: status, Tunnels: tunnels, TunnelPaths: tunnelPaths, Logs: s.logs.Snapshot(), Update: update}
}

func copyTunnelPaths(paths map[string]string) map[string]string {
	copy := make(map[string]string, len(paths))
	for tunnelID, path := range paths {
		copy[tunnelID] = path
	}
	return copy
}

func (s *DesktopService) SaveConfig(patch AppConfigPatch) (DesktopSnapshot, error) {
	s.mu.Lock()
	previous := s.cfg
	cfg := patch.Apply(previous)
	s.mu.Unlock()
	if err := s.store.Save(cfg); err != nil {
		return DesktopSnapshot{}, err
	}
	if err := SetAutoStart(cfg.AutoStart); err != nil {
		return DesktopSnapshot{}, err
	}
	s.mu.Lock()
	s.cfg = cfg
	restart := s.cancel != nil && connectionConfigChanged(previous, cfg)
	s.mu.Unlock()
	s.logs.append(LogEntry{Time: time.Now(), Level: "INFO", Message: "configuration saved"})
	if restart {
		_, _ = s.startConnection(true)
	}
	return s.Snapshot(), nil
}

func (s *DesktopService) Status() RuntimeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *DesktopService) CheckForUpdate() (UpdateResult, error) {
	return s.updater.CheckForUpdate()
}

func (s *DesktopService) ApplyUpdate() error {
	return s.updater.ApplyUpdate()
}

func (s *DesktopService) OpenURL(url string) error {
	s.mu.Lock()
	browser := s.browser
	s.mu.Unlock()
	if browser == nil {
		return fmt.Errorf("浏览器尚未初始化")
	}
	return browser.OpenURL(url)
}

func (s *DesktopService) ShowWindow() {
	s.mu.Lock()
	window := s.window
	s.mu.Unlock()
	if window != nil {
		window.Show()
	}
}

func (s *DesktopService) HideWindow() {
	s.mu.Lock()
	window := s.window
	s.mu.Unlock()
	if window != nil {
		window.Hide()
	}
}

func (s *DesktopService) Quit() {
	s.Disconnect()
	s.mu.Lock()
	window := s.window
	s.mu.Unlock()
	if window != nil {
		window.Quit()
	}
}
