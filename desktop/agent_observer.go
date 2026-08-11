package main

import "github.com/nrytex/nrynet/internal/model"

type desktopObserver struct {
	service *DesktopService
	runID   uint64
}

func (o desktopObserver) SessionStarted() {
	o.service.onSessionStarted(o.runID)
}

func (o desktopObserver) SessionEnded(err error) {
	o.service.onSessionEnded(o.runID, err)
}

func (o desktopObserver) TunnelSnapshot(tunnels []model.Tunnel) {
	o.service.onTunnelSnapshot(o.runID, tunnels)
}

func (o desktopObserver) Transfer(tunnelID, direction string, bytes int64) {
	o.service.onTransfer(o.runID, direction, bytes)
}

func (o desktopObserver) TunnelPath(tunnelID, path string) {
	o.service.onTunnelPath(o.runID, tunnelID, path)
}
