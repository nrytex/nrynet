package main

import "github.com/nat-link/nat-link/internal/model"

type desktopObserver struct {
	service *DesktopService
}

func (o desktopObserver) SessionStarted() {
	o.service.onSessionStarted()
}

func (o desktopObserver) SessionEnded(err error) {
	o.service.onSessionEnded(err)
}

func (o desktopObserver) TunnelSnapshot(tunnels []model.Tunnel) {
	o.service.onTunnelSnapshot(tunnels)
}

func (o desktopObserver) Transfer(tunnelID, direction string, bytes int64) {
	o.service.onTransfer(tunnelID, direction, bytes)
}
