package dns

import (
	"net/netip"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter applies global and per-client token buckets while bounding remembered clients.
type Limiter struct {
	mu                           sync.Mutex
	clients                      map[netip.Addr]bucket
	order                        []netip.Addr
	next                         int
	perSecond, burst             float64
	globalPerSecond, globalBurst float64
	maxClients                   int
	global                       bucket
	now                          func() time.Time
}

func NewLimiter(perSecond, burst, globalPerSecond, globalBurst, maxClients int) *Limiter {
	now := time.Now()
	return &Limiter{
		clients: make(map[netip.Addr]bucket, maxClients), perSecond: float64(perSecond), burst: float64(burst),
		globalPerSecond: float64(globalPerSecond), globalBurst: float64(globalBurst), maxClients: maxClients,
		global: bucket{tokens: float64(globalBurst), last: now}, now: time.Now,
	}
}

// Allow returns an empty reason on success and a bounded metric reason on rejection.
func (l *Limiter) Allow(client netip.Addr) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !take(&l.global, now, l.globalPerSecond, l.globalBurst) {
		return "global_rate"
	}
	client = client.Unmap()
	value, exists := l.clients[client]
	if !exists {
		if len(l.clients) >= l.maxClients {
			delete(l.clients, l.order[l.next])
			l.order[l.next] = client
			l.next = (l.next + 1) % l.maxClients
		} else {
			l.order = append(l.order, client)
		}
		value = bucket{tokens: l.burst, last: now}
	}
	if !take(&value, now, l.perSecond, l.burst) {
		l.clients[client] = value
		l.global.tokens++
		return "client_rate"
	}
	l.clients[client] = value
	return ""
}

func take(value *bucket, now time.Time, rate, burst float64) bool {
	if now.After(value.last) {
		value.tokens = min(burst, value.tokens+now.Sub(value.last).Seconds()*rate)
		value.last = now
	}
	if value.tokens < 1 {
		return false
	}
	value.tokens--
	return true
}
