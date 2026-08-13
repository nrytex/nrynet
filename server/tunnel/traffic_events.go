package tunnel

import (
	"context"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/storage"
)

type eventLimiter struct {
	mu      sync.Mutex
	window  time.Time
	count   int
	maxRate int
}

func (l *eventLimiter) allow() bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.window.IsZero() || now.Sub(l.window) >= time.Second {
		l.window = now
		l.count = 0
	}
	if l.count >= l.maxRate {
		return false
	}
	l.count++
	return true
}

type trafficEventRecorder struct {
	store   *storage.Store
	limiter eventLimiter
}

func newTrafficEventRecorder(store *storage.Store) *trafficEventRecorder {
	return &trafficEventRecorder{store: store, limiter: eventLimiter{maxRate: 50}}
}

func (r *trafficEventRecorder) record(ctx context.Context, level, event, message string, fields map[string]any) {
	if r == nil || r.store == nil || !r.limiter.allow() {
		return
	}
	_ = r.store.RecordEvent(ctx, level, event, message, fields)
}
