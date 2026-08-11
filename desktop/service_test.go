package main

import (
	"errors"
	"testing"

	"github.com/nrytex/nrynet/client/agent"
	"github.com/nrytex/nrynet/internal/model"
)

func TestDesktopObserverUpdatesSessionTunnelsAndTraffic(t *testing.T) {
	svc := &DesktopService{logs: newMemoryLogHandler(), status: RuntimeStatus{Version: appVersion}, cancel: func() {}, runID: 1}
	svc.onSessionStarted(1)
	if got := svc.Status(); !got.Connected || got.State != "connected" {
		t.Fatalf("unexpected started status: %+v", got)
	}
	tunnels := []model.Tunnel{{ID: "tun-1", Name: "web"}}
	svc.onTunnelSnapshot(1, tunnels)
	svc.onTunnelPath(1, "tun-1", "p2p")
	svc.onTransfer(1, agent.DirectionUpload, 12)
	svc.onTransfer(1, agent.DirectionDownload, 34)
	snap := svc.Snapshot()
	if len(snap.Tunnels) != 1 || snap.TunnelPaths["tun-1"] != "p2p" || snap.Status.UploadBytes != 12 || snap.Status.DownloadBytes != 34 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	svc.onSessionEnded(1, errors.New("closed"))
	if got := svc.Status(); got.Connected || got.State != "reconnecting" {
		t.Fatalf("unexpected ended status: %+v", got)
	}
}

func TestDesktopObserverIgnoresSessionEndAfterUserDisconnect(t *testing.T) {
	svc := &DesktopService{
		logs:   newMemoryLogHandler(),
		status: RuntimeStatus{State: "disconnected", Message: "已由用户断开连接。"},
	}
	svc.onSessionEnded(1, errors.New("connection reset"))
	if got := svc.Status(); got.State != "disconnected" || got.Message != "已由用户断开连接。" {
		t.Fatalf("late session end replaced intentional disconnect: %+v", got)
	}
}

func TestDesktopObserverIgnoresSupersededRun(t *testing.T) {
	svc := &DesktopService{
		logs: newMemoryLogHandler(), cancel: func() {}, runID: 2,
		status: RuntimeStatus{Connected: true, State: "connected"},
	}
	svc.onSessionEnded(1, errors.New("old connection closed"))
	if got := svc.Status(); !got.Connected || got.State != "connected" {
		t.Fatalf("superseded run changed current status: %+v", got)
	}
}

func TestConnectionConfigChangedForTokenButNotAutoStart(t *testing.T) {
	base := AppConfig{ServerURL: "wss://server/agent/connect", Token: "old"}
	rotated := base
	rotated.Token = "new"
	if !connectionConfigChanged(base, rotated) {
		t.Fatal("token rotation did not require a reconnect")
	}
	autoStart := base
	autoStart.AutoStart = true
	if connectionConfigChanged(base, autoStart) {
		t.Fatal("auto-start-only update required a reconnect")
	}
}
