package agent

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/config"
)

func TestOpenAndRelaySurvivesControlSessionCancellationDuringSetup(t *testing.T) {
	localAddress := startLocalEcho(t)
	dataAddress, done := startDataPeer(t)
	agent := &Agent{
		options: Options{Config: config.ClientConfig{
			DataAddress: dataAddress, Token: "token-1", DeviceID: "device-1",
		}},
		logger: slog.Default(),
	}
	runCtx, stopRun := context.WithCancel(context.Background())
	defer stopRun()
	agent.beginRun(runCtx)
	defer agent.endRun()
	sessionCtx, cancelSession := context.WithCancel(context.Background())
	cancelSession()
	if err := agent.openAndRelay(sessionCtx, testControl{agent: agent}, openMessage(t, "req-1", localAddress)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("data setup stopped with the control session")
	}
}
