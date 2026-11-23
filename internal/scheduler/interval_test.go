package scheduler

import (
	"testing"
	"time"
)

func TestTimerResetAlwaysUsesPositiveDuration(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Second, time.Nanosecond, time.Duration(1<<63 - 1)} {
		for range 20 {
			if got := jitteredInterval(interval); got <= 0 {
				t.Fatalf("jitteredInterval(%s) = %s", interval, got)
			}
		}
		ticker := jitteredTicker(interval)
		resetWithJitter(ticker, interval)
		ticker.Stop()
	}
}
