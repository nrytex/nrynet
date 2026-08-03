package agent

import (
	"context"
	"net"
	"testing"

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/model"
)

type recordingObserver struct {
	ended chan error
}

func (o recordingObserver) SessionStarted()                {}
func (o recordingObserver) SessionEnded(err error)         { o.ended <- err }
func (o recordingObserver) TunnelSnapshot([]model.Tunnel)  {}
func (o recordingObserver) Transfer(string, string, int64) {}

func TestRunSessionReportsInitialConnectionFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	observer := recordingObserver{ended: make(chan error, 1)}
	client, err := New(Options{
		Config: config.ClientConfig{
			ServerURL: "ws://" + address + "/agent/connect", DataAddress: address,
			Transport: "websocket", Token: "token", DeviceID: "device",
		},
		Observer: observer,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.runSession(context.Background()); err == nil {
		t.Fatal("expected the closed listener to reject the connection")
	}
	select {
	case observed := <-observer.ended:
		if observed == nil {
			t.Fatal("observer received a nil connection error")
		}
	default:
		t.Fatal("observer was not notified about the initial connection failure")
	}
}
