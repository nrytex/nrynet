package relay

import (
	"testing"
	"time"
)

func TestBandwidthMeterUsesRollingWindow(t *testing.T) {
	now := time.Unix(100, 0)
	meter := newBandwidthMeter()
	meter.clock = func() time.Time { return now }
	meter.Add(500)
	if got := meter.BytesPerSecond(); got != 100 {
		t.Fatalf("rate=%d want=100", got)
	}
	now = now.Add(5 * time.Second)
	if got := meter.BytesPerSecond(); got != 0 {
		t.Fatalf("expired rate=%d", got)
	}
}
