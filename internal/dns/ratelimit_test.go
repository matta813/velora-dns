package dns

import (
	"net/netip"
	"testing"
	"time"
)

func TestRateLimiterEnforcesPerClientAndGlobalBuckets(t *testing.T) {
	now := time.Unix(100, 0)
	l := NewRateLimiter(100, 1, 1)
	first := netip.MustParseAddr("192.0.2.1")
	if !l.Allow(first, now) || l.Allow(first, now) {
		t.Fatal("per-client rate limit did not reject")
	}
	if !l.Allow(first, now.Add(time.Second)) {
		t.Fatal("per-client bucket did not refill")
	}
	global := NewRateLimiter(1, 100, 2)
	if !global.Allow(first, now) || !global.Allow(netip.MustParseAddr("192.0.2.2"), now) || global.Allow(netip.MustParseAddr("192.0.2.3"), now) {
		t.Fatal("global rate limit did not reject")
	}
}

func TestRateLimiterBoundsClientTracking(t *testing.T) {
	l := NewRateLimiter(20_000, 20_000, 2)
	l.maxItems = 2
	now := time.Unix(100, 0)
	for _, ip := range []string{"192.0.2.1", "192.0.2.2"} {
		if !l.Allow(netip.MustParseAddr(ip), now) {
			t.Fatal("initial client rejected")
		}
	}
	if l.Allow(netip.MustParseAddr("192.0.2.3"), now) || len(l.clients) != 2 {
		t.Fatal("client table grew beyond cap")
	}
}

func TestRateLimiterPrunesInactiveClientsAtCapacity(t *testing.T) {
	l := NewRateLimiter(20_000, 20_000, 2)
	l.maxItems = 1
	now := time.Unix(100, 0)
	if !l.Allow(netip.MustParseAddr("192.0.2.1"), now) {
		t.Fatal("initial client rejected")
	}
	if !l.Allow(netip.MustParseAddr("192.0.2.2"), now.Add(11*time.Minute)) {
		t.Fatal("inactive client bucket was not pruned")
	}
}

func TestRateLimitStateReportsRejectionsWithoutClientAddresses(t *testing.T) {
	state := NewRateLimitState(true, 100, 1, 1)
	now := time.Now()
	client := netip.MustParseAddr("192.0.2.1")
	if !state.Allow(client, now) || state.Allow(client, now) {
		t.Fatal("expected one allowed query and one rejection")
	}
	status := state.Status()
	if !status.Enabled || status.RejectedTotal != 1 || status.LastRejectedAt == nil || !status.LastRejectedAt.Equal(now) {
		t.Fatalf("unexpected status: %+v", status)
	}
	state.Configure(false, 100, 1, 1)
	if !state.Allow(client, now) || state.Status().RejectedTotal != 1 {
		t.Fatal("disabled limiter must not count rejections")
	}
}
