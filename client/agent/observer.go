package agent

import "github.com/nrytex/nrynet/internal/model"

const (
	DirectionUpload   = "upload"
	DirectionDownload = "download"
)

type Observer interface {
	SessionStarted()
	SessionEnded(error)
	TunnelSnapshot([]model.Tunnel)
	Transfer(tunnelID, direction string, bytes int64)
}

type TunnelPathObserver interface {
	TunnelPath(tunnelID, path string)
}

func (a *Agent) notifySessionStarted() {
	if a.options.Observer != nil {
		a.options.Observer.SessionStarted()
	}
}

func (a *Agent) notifySessionEnded(err error) {
	if a.options.Observer != nil {
		a.options.Observer.SessionEnded(err)
	}
}

func (a *Agent) notifyTunnelSnapshot(tunnels []model.Tunnel) {
	if a.options.Observer != nil {
		a.options.Observer.TunnelSnapshot(tunnels)
	}
}

func (a *Agent) notifyTransfer(tunnelID, direction string, bytes int64) {
	if a.options.Observer != nil && bytes > 0 {
		a.options.Observer.Transfer(tunnelID, direction, bytes)
	}
}

func (a *Agent) notifyTunnelPath(tunnelID, path string) {
	observer, ok := a.options.Observer.(TunnelPathObserver)
	if ok {
		observer.TunnelPath(tunnelID, path)
	}
}
