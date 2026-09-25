package dns

import (
	"errors"
	"testing"
	"time"
)

func TestUpstreamHealthTripsAndRecoversAfterCooldown(t *testing.T) {
	health := NewUpstreamHealth([]string{"127.0.0.1:53", "127.0.0.2:53"})
	now := time.Unix(100, 0)
	first := "127.0.0.1:53"
	for i := 0; i < upstreamFailureThreshold; i++ {
		if !health.Begin(first, now) {
			t.Fatal("upstream blocked before failure threshold")
		}
		health.Finish(first, now, time.Second, errors.New("timeout"))
	}
	if health.Begin(first, now.Add(time.Second)) {
		t.Fatal("failed upstream was retried during cooldown")
	}
	if !health.Begin("127.0.0.2:53", now.Add(time.Second)) {
		t.Fatal("healthy fallback was blocked")
	}
	if !health.Begin(first, now.Add(upstreamRetryDelay)) || health.Begin(first, now.Add(upstreamRetryDelay)) {
		t.Fatal("expected exactly one recovery probe")
	}
	health.Finish(first, now.Add(upstreamRetryDelay), 5*time.Millisecond, nil)
	status := health.Snapshot()
	if len(status) != 2 || status[0].State != "healthy" || status[0].ConsecutiveFailures != 0 || status[0].LatencyMilliseconds != 5 {
		t.Fatalf("recovery state: %+v", status)
	}
}
