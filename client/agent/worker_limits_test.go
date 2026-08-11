package agent

import (
	"context"
	"testing"
)

func TestStreamWorkerLimitReleases(t *testing.T) {
	agent := &Agent{}
	ctx := context.Background()
	for index := 0; index < maxActiveStreamWorkers; index++ {
		if !agent.acquireStreamWorker(ctx) {
			t.Fatalf("stream worker %d was rejected before capacity was full", index)
		}
	}
	blockedCtx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- agent.acquireStreamWorker(blockedCtx) }()
	cancel()
	if <-done {
		t.Fatal("stream worker limit allowed an extra worker")
	}
	for index := 0; index < maxActiveStreamWorkers; index++ {
		agent.releaseStreamWorker()
	}
	if !agent.acquireStreamWorker(ctx) {
		t.Fatal("stream worker slot was not released")
	}
	agent.releaseStreamWorker()
}

func TestVisitorSessionLimitReleases(t *testing.T) {
	agent := &Agent{}
	for index := 0; index < maxActiveVisitorSessions; index++ {
		if !agent.acquireVisitorSession() {
			t.Fatalf("visitor session %d was rejected before capacity was full", index)
		}
	}
	if agent.acquireVisitorSession() {
		t.Fatal("visitor session limit allowed an extra session")
	}
	for index := 0; index < maxActiveVisitorSessions; index++ {
		agent.releaseVisitorSession()
	}
	if !agent.acquireVisitorSession() {
		t.Fatal("visitor session slot was not released")
	}
	agent.releaseVisitorSession()
}
