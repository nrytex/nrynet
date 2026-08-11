package agent

import (
	"log/slog"
	"testing"

	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
)

type pathMessageObserver struct {
	path     string
	tunnelID string
}

func (o *pathMessageObserver) SessionStarted()                {}
func (o *pathMessageObserver) SessionEnded(error)             {}
func (o *pathMessageObserver) TunnelSnapshot([]model.Tunnel)  {}
func (o *pathMessageObserver) Transfer(string, string, int64) {}
func (o *pathMessageObserver) TunnelPath(tunnelID, path string) {
	o.tunnelID = tunnelID
	o.path = path
}

func TestHandleTunnelPathNotifiesOptionalObserver(t *testing.T) {
	observer := &pathMessageObserver{}
	a := &Agent{options: Options{Observer: observer}, logger: slog.Default()}
	message, err := protocol.NewMessage(protocol.TypeTunnelPath, "", "tunnel-1", protocol.TunnelPathPayload{Path: protocol.TunnelPathP2P})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.handleTunnelPath(message); err != nil {
		t.Fatal(err)
	}
	if observer.tunnelID != "tunnel-1" || observer.path != protocol.TunnelPathP2P {
		t.Fatalf("observer received tunnel=%q path=%q", observer.tunnelID, observer.path)
	}
}
