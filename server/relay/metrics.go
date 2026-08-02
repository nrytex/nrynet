package relay

import (
	"sync"
	"time"
)

const bandwidthWindowSeconds = int64(5)

type bandwidthMeter struct {
	mu      sync.Mutex
	clock   func() time.Time
	buckets map[int64]int64
}

func newBandwidthMeter() *bandwidthMeter {
	return &bandwidthMeter{clock: time.Now, buckets: make(map[int64]int64)}
}

func (m *bandwidthMeter) Add(bytes int64) {
	if bytes <= 0 {
		return
	}
	now := m.clock().Unix()
	m.mu.Lock()
	m.buckets[now] += bytes
	m.prune(now)
	m.mu.Unlock()
}

func (m *bandwidthMeter) BytesPerSecond() int64 {
	now := m.clock().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prune(now)
	var total int64
	for _, bytes := range m.buckets {
		total += bytes
	}
	return total / bandwidthWindowSeconds
}

func (m *bandwidthMeter) prune(now int64) {
	oldest := now - bandwidthWindowSeconds + 1
	for second := range m.buckets {
		if second < oldest {
			delete(m.buckets, second)
		}
	}
}
