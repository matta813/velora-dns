package dns

import (
	"sync"
	"time"
)

const upstreamFailureThreshold = 3
const upstreamRetryDelay = 30 * time.Second

type UpstreamStatus struct {
	Address             string     `json:"address"`
	State               string     `json:"state"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LatencyMilliseconds float64    `json:"latency_milliseconds"`
	LastSuccess         *time.Time `json:"last_success,omitempty"`
	LastFailure         *time.Time `json:"last_failure,omitempty"`
}

type upstreamState struct {
	status  UpstreamStatus
	retryAt time.Time
	probing bool
}

// UpstreamHealth tracks only configured upstreams. Failed destinations receive
// one trial request after a cooldown, avoiding repeated timeouts on every query.
type UpstreamHealth struct {
	mu    sync.Mutex
	order []string
	items map[string]*upstreamState
}

func NewUpstreamHealth(addresses []string) *UpstreamHealth {
	h := &UpstreamHealth{order: append([]string(nil), addresses...), items: make(map[string]*upstreamState, len(addresses))}
	for _, address := range addresses {
		h.items[address] = &upstreamState{status: UpstreamStatus{Address: address, State: "unknown"}}
	}
	return h
}

func (h *UpstreamHealth) Begin(address string, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	item, ok := h.items[address]
	if !ok {
		return false
	}
	if item.status.ConsecutiveFailures < upstreamFailureThreshold {
		return true
	}
	if now.Before(item.retryAt) || item.probing {
		return false
	}
	item.probing = true
	return true
}

func (h *UpstreamHealth) Finish(address string, now time.Time, duration time.Duration, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	item, ok := h.items[address]
	if !ok {
		return
	}
	item.probing = false
	if err == nil {
		item.status.State = "healthy"
		item.status.ConsecutiveFailures = 0
		item.status.LatencyMilliseconds = float64(duration) / float64(time.Millisecond)
		item.status.LastSuccess = &now
		item.retryAt = time.Time{}
		return
	}
	item.status.ConsecutiveFailures++
	item.status.LastFailure = &now
	if item.status.ConsecutiveFailures >= upstreamFailureThreshold {
		item.status.State = "unavailable"
		item.retryAt = now.Add(upstreamRetryDelay)
	} else {
		item.status.State = "degraded"
	}
}

func (h *UpstreamHealth) Snapshot() []UpstreamStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]UpstreamStatus, 0, len(h.order))
	for _, address := range h.order {
		out = append(out, h.items[address].status)
	}
	return out
}
