package tunnel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nrytex/nrynet/internal/storage"
)

func TestConnectionEventsSkipNormalClosuresAndDescribeFailures(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/connection-events.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	manager := &Manager{store: store}
	manager.recordConnectionFailure("tunnel", "normal", nil)
	manager.recordConnectionFailure("tunnel", "failed", errors.New("data channel timeout"))

	events, err := store.ListEvents(context.Background(), storage.EventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event != "connection.failed" ||
		!strings.Contains(events[0].Message, "data channel timeout") {
		t.Fatalf("unexpected connection events: %+v", events)
	}
}
