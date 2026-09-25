package dns

import (
	"net/netip"
	"sync"
	"time"
)

// RateLimiter applies bounded token buckets globally and per client. Its client
// table is capped, so a spoofed-source UDP flood cannot grow process memory.
type RateLimiter struct {
	mu         sync.Mutex
	global     bucket
	clients    map[netip.Addr]bucket
	globalRate float64
	perRate    float64
	burst      float64
	maxItems   int
}

// RateLimitState is a live-updatable wrapper around an optional RateLimiter.
// The management API swaps the limiter and enabled flag at runtime.
type RateLimitState struct {
	mu             sync.Mutex
	enabled        bool
	limiter        *RateLimiter
	rejectedTotal  uint64
	lastRejectedAt time.Time
}

type RateLimitStatus struct {
	Enabled        bool       `json:"enabled"`
	RejectedTotal  uint64     `json:"rejected_total"`
	LastRejectedAt *time.Time `json:"last_rejected_at,omitempty"`
}

func NewRateLimitState(enabled bool, globalQPS, clientQPS, burst int) *RateLimitState {
	return &RateLimitState{enabled: enabled, limiter: NewRateLimiter(globalQPS, clientQPS, burst)}
}

func (s *RateLimitState) Configure(enabled bool, globalQPS, clientQPS, burst int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	s.limiter = NewRateLimiter(globalQPS, clientQPS, burst)
}

func (s *RateLimitState) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

func (s *RateLimitState) Allow(client netip.Addr, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		return true
	}
	if s.limiter.Allow(client, now) {
		return true
	}
	s.rejectedTotal++
	s.lastRejectedAt = now
	return false
}

func (s *RateLimitState) Status() RateLimitStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := RateLimitStatus{Enabled: s.enabled, RejectedTotal: s.rejectedTotal}
	if !s.lastRejectedAt.IsZero() {
		last := s.lastRejectedAt
		status.LastRejectedAt = &last
	}
	return status
}

func (s *RateLimitState) RestoreRejections(total uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectedTotal = total
	s.lastRejectedAt = time.Time{}
}

func (s *RateLimitState) ResetRejections() {
	s.RestoreRejections(0)
}

type bucket struct {
	tokens  float64
	updated time.Time
}

func NewRateLimiter(globalQPS, clientQPS, burst int) *RateLimiter {
	now := time.Now()
	return &RateLimiter{global: bucket{tokens: float64(burst), updated: now}, clients: make(map[netip.Addr]bucket), globalRate: float64(globalQPS), perRate: float64(clientQPS), burst: float64(burst), maxItems: 10_000}
}

func (l *RateLimiter) Allow(client netip.Addr, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !take(&l.global, l.globalRate, l.burst, now) {
		return false
	}
	b, found := l.clients[client]
	if !found && len(l.clients) >= l.maxItems {
		// Prune inactive buckets before rejecting a new source. This keeps the
		// allocation bound while allowing a legitimate client to return after a
		// broad but short-lived source-address flood.
		for address, candidate := range l.clients {
			if now.Sub(candidate.updated) > 10*time.Minute {
				delete(l.clients, address)
			}
		}
		if len(l.clients) >= l.maxItems {
			return false
		}
	}
	if !take(&b, l.perRate, l.burst, now) {
		return false
	}
	l.clients[client] = b
	return true
}

func take(b *bucket, rate, burst float64, now time.Time) bool {
	if !b.updated.IsZero() {
		if now.After(b.updated) {
			b.tokens = min(burst, b.tokens+now.Sub(b.updated).Seconds()*rate)
		}
	} else {
		b.tokens = burst
	}
	b.updated = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
