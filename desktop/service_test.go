package main

import (
	"errors"
	"testing"

	"github.com/nat-link/nat-link/client/agent"
	"github.com/nat-link/nat-link/internal/model"
)

func TestDesktopObserverUpdatesSessionTunnelsAndTraffic(t *testing.T) {
	svc := &DesktopService{logs: newMemoryLogHandler(), status: RuntimeStatus{Version: appVersion}, cancel: func() {}}
	svc.onSessionStarted()
	if got := svc.Status(); !got.Connected || got.State != "connected" {
		t.Fatalf("unexpected started status: %+v", got)
	}
	tunnels := []model.Tunnel{{ID: "tun-1", Name: "web"}}
	svc.onTunnelSnapshot(tunnels)
	svc.onTransfer("tun-1", agent.DirectionUpload, 12)
	svc.onTransfer("tun-1", agent.DirectionDownload, 34)
	snap := svc.Snapshot()
	if len(snap.Tunnels) != 1 || snap.Status.UploadBytes != 12 || snap.Status.DownloadBytes != 34 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	svc.onSessionEnded(errors.New("closed"))
	if got := svc.Status(); got.Connected || got.State != "reconnecting" {
		t.Fatalf("unexpected ended status: %+v", got)
	}
}

func TestDesktopObserverIgnoresSessionEndAfterUserDisconnect(t *testing.T) {
	svc := &DesktopService{
		logs:   newMemoryLogHandler(),
		status: RuntimeStatus{State: "disconnected", Message: "已由用户断开连接。"},
	}
	svc.onSessionEnded(errors.New("connection reset"))
	if got := svc.Status(); got.State != "disconnected" || got.Message != "已由用户断开连接。" {
		t.Fatalf("late session end replaced intentional disconnect: %+v", got)
	}
}
