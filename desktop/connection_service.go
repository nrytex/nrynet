package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/nrytex/nrynet/client/agent"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/model"
)

func (s *DesktopService) Connect() (RuntimeStatus, error) {
	return s.startConnection(false)
}

func (s *DesktopService) startConnection(restart bool) (RuntimeStatus, error) {
	s.mu.Lock()
	if s.cancel != nil && !restart {
		status := s.status
		s.mu.Unlock()
		return status, nil
	}
	if restart && s.cancel == nil {
		status := s.status
		s.mu.Unlock()
		return status, nil
	}
	previousCancel := s.cancel
	s.runID++
	runID := s.runID
	cfg := s.cfg
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.status = RuntimeStatus{
		Connected: false, State: "connecting", Version: appVersion, LastStartedAt: time.Now(),
	}
	s.mu.Unlock()
	if previousCancel != nil {
		previousCancel()
	}

	client, err := s.newAgent(cfg, runID)
	if err != nil {
		cancel()
		s.logs.append(LogEntry{
			Time: time.Now(), Level: "ERROR", Message: "connection configuration invalid",
			Fields: map[string]any{"error": err.Error()},
		})
		message := connectionErrorMessage(err)
		s.setStopped(runID, message)
		return s.Status(), errors.New(message)
	}
	go s.runAgent(ctx, client, runID)
	return s.Status(), nil
}

func (s *DesktopService) Disconnect() RuntimeStatus {
	s.mu.Lock()
	cancel := s.cancel
	s.runID++
	s.cancel = nil
	s.setStoppedLocked("已由用户断开连接。")
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return s.Status()
}

func (s *DesktopService) newAgent(cfg AppConfig, runID uint64) (*agent.Agent, error) {
	opts := agent.Options{
		Config: cfg.toClientConfig(), Version: appVersion,
		HeartbeatInterval: 15 * time.Second, ReconnectMin: time.Second,
		ReconnectMax: 30 * time.Second, Observer: desktopObserver{service: s, runID: runID},
	}
	normal := agent.NewOptions(config.Config{Client: opts.Config}, appVersion)
	opts.Config = normal.Config
	return agent.New(opts, slog.New(s.logs))
}

func (s *DesktopService) runAgent(ctx context.Context, client *agent.Agent, runID uint64) {
	err := s.runAgentSafely(ctx, client)
	if errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	message := "connected"
	if err != nil {
		message = err.Error()
	}
	s.setStopped(runID, message)
}

func (s *DesktopService) runAgentSafely(ctx context.Context, client *agent.Agent) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logs.append(LogEntry{
				Time: time.Now(), Level: "ERROR", Message: "agent panic recovered",
				Fields: map[string]any{"panic": recovered, "stack": string(debug.Stack())},
			})
			err = fmt.Errorf("agent panic: %v", recovered)
		}
	}()
	return client.Run(ctx)
}

func (s *DesktopService) setStopped(runID uint64, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if runID != s.runID {
		return
	}
	s.cancel = nil
	s.setStoppedLocked(message)
}

func (s *DesktopService) setStoppedLocked(message string) {
	upload, download := s.status.UploadBytes, s.status.DownloadBytes
	s.tunnelPaths = make(map[string]string)
	s.status = RuntimeStatus{
		Connected: false, State: "disconnected", Message: message,
		Version: appVersion, UploadBytes: upload, DownloadBytes: download,
		LastStoppedAt: time.Now(),
	}
}

func (s *DesktopService) onSessionStarted(runID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if runID != s.runID || s.cancel == nil {
		return
	}
	s.status.Connected = true
	s.status.State = "connected"
	s.tunnelPaths = make(map[string]string)
	s.status.Message = "已连接并通过身份验证。"
}

func (s *DesktopService) onSessionEnded(runID uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if runID != s.runID || s.cancel == nil {
		return
	}
	s.status.Connected = false
	s.status.State = "reconnecting"
	s.tunnelPaths = make(map[string]string)
	s.status.Message = "连接已中断，客户端正在尝试重新连接。"
	if err != nil {
		s.status.Message = connectionErrorMessage(err)
	}
}

func (s *DesktopService) onTunnelSnapshot(runID uint64, tunnels []model.Tunnel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if runID != s.runID || s.cancel == nil {
		return
	}
	s.tunnels = append([]model.Tunnel{}, tunnels...)
	known := make(map[string]struct{}, len(tunnels))
	for _, tunnel := range tunnels {
		known[tunnel.ID] = struct{}{}
	}
	for tunnelID := range s.tunnelPaths {
		if _, ok := known[tunnelID]; !ok {
			delete(s.tunnelPaths, tunnelID)
		}
	}
}

func (s *DesktopService) onTunnelPath(runID uint64, tunnelID, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if runID != s.runID || s.cancel == nil || tunnelID == "" || path == "" {
		return
	}
	if s.tunnelPaths == nil {
		s.tunnelPaths = make(map[string]string)
	}
	s.tunnelPaths[tunnelID] = path
}

func (s *DesktopService) onTransfer(runID uint64, direction string, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if runID != s.runID || s.cancel == nil {
		return
	}
	if direction == agent.DirectionUpload {
		s.status.UploadBytes += bytes
		return
	}
	if direction == agent.DirectionDownload {
		s.status.DownloadBytes += bytes
	}
}

func connectionConfigChanged(previous, next AppConfig) bool {
	return previous.ServerURL != next.ServerURL ||
		previous.DataAddress != next.DataAddress ||
		previous.Transport != next.Transport ||
		previous.QUICAddress != next.QUICAddress ||
		previous.CAFile != next.CAFile ||
		previous.Token != next.Token ||
		previous.Name != next.Name ||
		previous.DeviceID != next.DeviceID ||
		previous.InsecureSkipVerify != next.InsecureSkipVerify
}
