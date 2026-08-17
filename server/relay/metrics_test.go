package relay

import (
	"testing"
	"time"
)

func TestBandwidthMeterReportsSampledRateWithoutFiveSecondDilution(t *testing.T) {
	start := time.Unix(100, 0)
	now := start
	meter := &bandwidthMeter{clock: func() time.Time { return now }, lastSample: start}
	meter.Add(64 * 1024)
	now = now.Add(time.Second)
	if got := meter.BytesPerSecond(); got != 64*1024 {
		t.Fatalf("rate=%d, want %d", got, 64*1024)
	}
}

func TestBandwidthMeterKeepsLastRateBetweenSamples(t *testing.T) {
	start := time.Unix(200, 0)
	now := start
	meter := &bandwidthMeter{clock: func() time.Time { return now }, lastSample: start}
	meter.Add(1000)
	now = now.Add(time.Second)
	if got := meter.BytesPerSecond(); got != 1000 {
		t.Fatalf("first rate=%d", got)
	}
	meter.Add(1000)
	now = now.Add(100 * time.Millisecond)
	if got := meter.BytesPerSecond(); got != 1000 {
		t.Fatalf("rate inside sample window=%d", got)
	}
}
