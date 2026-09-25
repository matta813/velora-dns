package metrics

import (
	"net/http"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/querylog"
	wire "github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Snapshot struct {
	Queries      uint64  `json:"queries_total"`
	Blocked      uint64  `json:"blocked_queries"`
	QPS          float64 `json:"queries_per_second"`
	CacheHitRate float64 `json:"cache_hit_rate"`
}

func (m *Metrics) ObserveQueryLog(log *querylog.Logger) {
	m.registry.MustRegister(
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "dns_query_log_dropped_total", Help: "Query log records dropped on queue overflow or storage failure."}, func() float64 { return float64(log.Snapshot().Dropped) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "dns_query_log_errors_total", Help: "Failed history writes and retention operations."}, func() float64 { return float64(log.Snapshot().Errors) }),
	)
}

type Metrics struct {
	registry         *prometheus.Registry
	queries          *prometheus.CounterVec
	duration         prometheus.Histogram
	requests, errors *prometheus.CounterVec
	overload         *prometheus.CounterVec
	mu               sync.Mutex
	total            uint64
	blocked          uint64
	upstreamRequests map[string]uint64
	upstreamErrors   map[string]uint64
	buckets          [60]uint64
	seconds          [60]int64
	cache            *cache.Cache
}

func New(c *cache.Cache) *Metrics {
	r := prometheus.NewRegistry()
	m := &Metrics{registry: r, cache: c, upstreamRequests: make(map[string]uint64), upstreamErrors: make(map[string]uint64)}
	m.queries = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dns_queries_total", Help: "DNS requests by bounded type, source and response code."}, []string{"type", "source", "rcode"})
	m.duration = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "dns_query_duration_seconds", Help: "DNS request duration.", Buckets: prometheus.DefBuckets})
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dns_upstream_requests_total", Help: "Upstream attempts including failures."}, []string{"upstream"})
	m.errors = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dns_upstream_errors_total", Help: "Failed upstream attempts."}, []string{"upstream"})
	m.overload = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dns_overload_rejections_total", Help: "DNS requests or TCP connections rejected due to bounded resources."}, []string{"reason"})
	r.MustRegister(m.queries, m.duration, m.requests, m.errors, m.overload, prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "dns_cache_entries", Help: "Live cache entries."}, func() float64 { return float64(c.Stats().Entries) }), prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "dns_cache_hits_total", Help: "Cache hits."}, func() float64 { return float64(c.Stats().Hits) }), prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "dns_cache_misses_total", Help: "Cache misses."}, func() float64 { return float64(c.Stats().Misses) }), prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "dns_queries_blocked_total", Help: "Queries blocked by DNS policy."}, func() float64 { m.mu.Lock(); defer m.mu.Unlock(); return float64(m.blocked) }))
	return m
}
func (m *Metrics) Query(kind, source string, rcode int, elapsed time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	code, ok := wire.RcodeToString[rcode]
	if !ok {
		code = "other"
	}
	m.queries.WithLabelValues(kind, source, code).Inc()
	m.duration.Observe(elapsed.Seconds())
	m.total++
	if source == "blocked" {
		m.blocked++
	}
	now := time.Now().Unix()
	i := now % 60
	if m.seconds[i] != now {
		m.seconds[i] = now
		m.buckets[i] = 0
	}
	m.buckets[i]++
}
func (m *Metrics) Upstream(server string, failed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests.WithLabelValues(server).Inc()
	m.upstreamRequests[server]++
	m.errors.WithLabelValues(server).Add(0)
	if failed {
		m.errors.WithLabelValues(server).Inc()
		m.upstreamErrors[server]++
	}
}
func (m *Metrics) Overload(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.overload.WithLabelValues(reason).Inc()
}

func (m *Metrics) Restore(queries, blocked uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total, m.blocked = queries, blocked
}

func (m *Metrics) UpstreamCounters() (map[string]uint64, map[string]uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests, errors := make(map[string]uint64, len(m.upstreamRequests)), make(map[string]uint64, len(m.upstreamErrors))
	for server, count := range m.upstreamRequests {
		requests[server] = count
	}
	for server, count := range m.upstreamErrors {
		errors[server] = count
	}
	return requests, errors
}

func (m *Metrics) RestoreUpstreamCounters(requests, errors map[string]uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for server, count := range requests {
		m.upstreamRequests[server] = count
		m.requests.WithLabelValues(server).Add(float64(count))
	}
	for server, count := range errors {
		m.upstreamErrors[server] = count
		m.errors.WithLabelValues(server).Add(float64(count))
	}
}

func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total, m.blocked = 0, 0
	clear(m.upstreamRequests)
	clear(m.upstreamErrors)
	m.buckets = [60]uint64{}
	m.seconds = [60]int64{}
	m.queries.Reset()
	m.requests.Reset()
	m.errors.Reset()
	m.overload.Reset()
}
func (m *Metrics) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Unix()
	var recent uint64
	for i, n := range m.buckets {
		if now-m.seconds[i] < 60 {
			recent += n
		}
	}
	c := m.cache.Stats()
	rate := 0.0
	if c.Hits+c.Misses > 0 {
		rate = float64(c.Hits) / float64(c.Hits+c.Misses)
	}
	return Snapshot{Queries: m.total, Blocked: m.blocked, QPS: float64(recent) / 60, CacheHitRate: rate}
}
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{MaxRequestsInFlight: 5, Timeout: 5 * time.Second})
}
