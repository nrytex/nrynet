package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nrytex/nrynet/client/agent"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/model"
)

type DesktopService struct {
	mu      sync.Mutex
	store   *fileStore
	cfg     AppConfig
	status  RuntimeStatus
	logs    *memoryLogHandler
	cancel  context.CancelFunc
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
	cfg := patch.Apply(s.cfg)
	s.mu.Unlock()
	if err := s.store.Save(cfg); err != nil {
		return DesktopSnapshot{}, err
	}
	if err := SetAutoStart(cfg.AutoStart); err != nil {
		return DesktopSnapshot{}, err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	s.logs.append(LogEntry{Time: time.Now(), Level: "INFO", Message: "configuration saved"})
	return s.Snapshot(), nil
}

func (s *DesktopService) Connect() (RuntimeStatus, error) {
	s.mu.Lock()
	if s.cancel != nil {
		defer s.mu.Unlock()
		return s.status, nil
	}
	cfg := s.cfg
	s.status = RuntimeStatus{Connected: false, State: "connecting", Version: appVersion, LastStartedAt: time.Now()}
	s.mu.Unlock()
	client, err := s.newAgent(cfg)
	if err != nil {
		s.logs.append(LogEntry{Time: time.Now(), Level: "ERROR", Message: "connection configuration invalid", Fields: map[string]any{"error": err.Error()}})
		message := connectionErrorMessage(err)
		s.setStopped(message)
		return s.Status(), errors.New(message)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	go s.runAgent(ctx, client)
	return s.Status(), nil
}

func (s *DesktopService) Disconnect() RuntimeStatus {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.setStopped("已由用户断开连接。")
	return s.Status()
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

func (s *DesktopService) newAgent(cfg AppConfig) (*agent.Agent, error) {
	opts := agent.Options{
		Config: cfg.toClientConfig(), Version: appVersion,
		HeartbeatInterval: 15 * time.Second, ReconnectMin: time.Second,
		ReconnectMax: 30 * time.Second, Observer: desktopObserver{s},
	}
	normal := agent.NewOptions(config.Config{Client: opts.Config}, appVersion)
	opts.Config = normal.Config
	return agent.New(opts, slog.New(s.logs))
}

func (s *DesktopService) runAgent(ctx context.Context, client *agent.Agent) {
	err := client.Run(ctx)
	if errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	msg := "connected"
	if err != nil {
		msg = err.Error()
	}
	s.setStopped(msg)
}

func (s *DesktopService) setStopped(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	upload, download := s.status.UploadBytes, s.status.DownloadBytes
	s.cancel = nil
	s.status = RuntimeStatus{
		Connected: false, State: "disconnected", Message: message,
		Version: appVersion, UploadBytes: upload, DownloadBytes: download,
		LastStoppedAt: time.Now(),
	}
}

func (s *DesktopService) onSessionStarted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Connected = true
	s.status.State = "connected"
	s.status.Message = "已连接并通过身份验证。"
}

func (s *DesktopService) onSessionEnded(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel == nil {
		return
	}
	s.status.Connected = false
	s.status.State = "reconnecting"
	s.status.Message = "连接已中断，客户端正在尝试重新连接。"
	if err != nil {
		s.status.Message = connectionErrorMessage(err)
	}
}

func (s *DesktopService) onTunnelSnapshot(tunnels []model.Tunnel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tunnels = append([]model.Tunnel{}, tunnels...)
}

func (s *DesktopService) onTransfer(_ string, direction string, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if direction == agent.DirectionUpload {
		s.status.UploadBytes += bytes
		return
	}
	if direction == agent.DirectionDownload {
		s.status.DownloadBytes += bytes
	}
}
