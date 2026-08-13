package tunnel

import (
	"testing"
	"time"
)

func TestEventLimiterAllowsConfiguredRateAndResetsWindow(t *testing.T) {
	limiter := eventLimiter{window: time.Now(), maxRate: 2}
	if !limiter.allow() || !limiter.allow() {
		t.Fatal("limiter rejected events below the configured rate")
	}
	if limiter.allow() {
		t.Fatal("limiter allowed an event above the configured rate")
	}
	limiter.window = time.Now().Add(-time.Second)
	if !limiter.allow() {
		t.Fatal("limiter did not reset after the one-second window")
	}
}

func TestEventLimiterWithZeroRateDoesNotPanic(t *testing.T) {
	limiter := eventLimiter{maxRate: 0}
	if limiter.allow() {
		t.Fatal("zero-rate limiter unexpectedly allowed an event")
	}
}
