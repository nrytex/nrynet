package tunnel

import (
	"context"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/storage"
)

const trafficFlushInterval = time.Second

type trafficDelta struct {
	upload   int64
	download int64
}

type trafficRecorder struct {
	store *storage.Store
	mu    sync.Mutex
	items map[string]trafficDelta
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
}

func newTrafficRecorder(store *storage.Store) *trafficRecorder {
	recorder := &trafficRecorder{
		store: store,
		items: make(map[string]trafficDelta),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	go recorder.run()
	return recorder
}

func (r *trafficRecorder) add(tunnelID string, upload, download int64) {
	if r == nil || tunnelID == "" || (upload <= 0 && download <= 0) {
		return
	}
	r.mu.Lock()
	delta := r.items[tunnelID]
	if upload > 0 {
		delta.upload += upload
	}
	if download > 0 {
		delta.download += download
	}
	r.items[tunnelID] = delta
	r.mu.Unlock()
}

func (r *trafficRecorder) run() {
	ticker := time.NewTicker(trafficFlushInterval)
	defer ticker.Stop()
	defer close(r.done)
	for {
		select {
		case <-ticker.C:
			r.flush()
		case <-r.stop:
			r.flush()
			return
		}
	}
}

func (r *trafficRecorder) flush() {
	r.mu.Lock()
	if len(r.items) == 0 {
		r.mu.Unlock()
		return
	}
	items := r.items
	r.items = make(map[string]trafficDelta, len(items))
	r.mu.Unlock()
	deltas := make([]storage.TrafficDelta, 0, len(items))
	for tunnelID, delta := range items {
		deltas = append(deltas, storage.TrafficDelta{TunnelID: tunnelID, Upload: delta.upload, Download: delta.download})
	}
	_ = r.store.RecordTrafficBatch(context.Background(), deltas)
}

func (r *trafficRecorder) close() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		close(r.stop)
		<-r.done
	})
}
