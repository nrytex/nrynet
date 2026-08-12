package main

import (
	"errors"
	"log/slog"
	"testing"
)

func TestMemoryLogHandlerStoresErrorsAsReadableText(t *testing.T) {
	logs := newMemoryLogHandler()
	slog.New(logs).Warn("control session ended", "error", errors.New("read QUIC control frame: connection reset"))

	entries := logs.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("log entries=%d, want 1", len(entries))
	}
	if got := entries[0].Fields["error"]; got != "read QUIC control frame: connection reset" {
		t.Fatalf("error field=%#v, want readable error text", got)
	}
}
