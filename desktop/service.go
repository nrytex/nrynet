package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/model"
)

type DesktopService struct {
	mu      sync.Mutex
	store   *fileStore
	cfg     AppConfig
	status  RuntimeStatus
	logs    *memoryLogHandler
	cancel  context.CancelFunc
	runID   uint64
	updater *UpdateService
	tunnels []model.Tunnel
	window  windowControls
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
	return &DesktopService{
		store: store, cfg: cfg, logs: logs, updater: updater,
		status:  RuntimeStatus{State: "disconnected", Version: appVersion},
		tunnels: make([]model.Tunnel, 0),
	}, nil
}

func (s *DesktopService) setWindow(window windowControls) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.window = window
}

func (s *DesktopService) Snapshot() DesktopSnapshot {
	s.mu.Lock()
	cfg, status := s.cfg, s.status
	tunnels := append([]model.Tunnel{}, s.tunnels...)
	s.mu.Unlock()
	return DesktopSnapshot{Config: cfg, Status: status, Tunnels: tunnels, Logs: s.logs.Snapshot()}
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
	return s.updater.CheckAndInstall()
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
