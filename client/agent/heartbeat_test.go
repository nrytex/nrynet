package agent

import (
	"context"
	"testing"
	"time"
)

func TestWaitHeartbeatAckIgnoresAnotherRequest(t *testing.T) {
	acks := make(chan string, 2)
	acks <- "heartbeat-old"
	acks <- "heartbeat-current"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := waitHeartbeatAck(ctx, time.Second, "heartbeat-current", acks); err != nil {
		t.Fatalf("waitHeartbeatAck() error = %v", err)
	}
}

func TestHeartbeatAckEnablesLivenessChecksAfterFirstAck(t *testing.T) {
	agent := &Agent{}
	acks := make(chan string, 1)
	agent.setHeartbeatAcks(acks)
	defer agent.clearHeartbeatAcks(acks)
	if agent.heartbeatAckRequired() {
		t.Fatal("heartbeat acknowledgement checks enabled before server response")
	}

	agent.enableHeartbeatAck()
	if !agent.heartbeatAckRequired() {
		t.Fatal("heartbeat acknowledgement checks were not enabled")
	}
}
