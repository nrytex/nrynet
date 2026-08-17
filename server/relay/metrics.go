package relay

import (
	"sync"
	"sync/atomic"
	"time"
)

const bandwidthMinimumSample = 250 * time.Millisecond

type bandwidthMeter struct {
	total atomic.Int64
	clock func() time.Time

	mu         sync.Mutex
	lastSample time.Time
	lastBytes  int64
	lastRate   int64
}

func newBandwidthMeter() *bandwidthMeter {
	now := time.Now()
	return &bandwidthMeter{clock: time.Now, lastSample: now}
}

func (m *bandwidthMeter) Add(bytes int64) {
	if bytes > 0 {
		m.total.Add(bytes)
	}
}

func (m *bandwidthMeter) BytesPerSecond() int64 {
	now := m.clock()
	total := m.total.Load()
	m.mu.Lock()
	defer m.mu.Unlock()
	elapsed := now.Sub(m.lastSample)
	if elapsed < bandwidthMinimumSample {
		return m.lastRate
	}
	delta := total - m.lastBytes
	if delta < 0 {
		delta = 0
	}
	m.lastRate = int64(float64(delta) / elapsed.Seconds())
	m.lastSample = now
	m.lastBytes = total
	return m.lastRate
}
