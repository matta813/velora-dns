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
	mu               sync.Mutex
	total            uint64
	blocked          uint64
	blockedCounter   prometheus.Counter
	overload         *prometheus.CounterVec
	tcpConnections   prometheus.Gauge
	buckets          [60]uint64
	seconds          [60]int64
	cache            *cache.Cache
}

func New(c *cache.Cache) *Metrics {
	r := prometheus.NewRegistry()
	m := &Metrics{registry: r, cache: c}
	m.queries = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dns_queries_total", Help: "DNS requests by bounded type, source and response code."}, []string{"type", "source", "rcode"})
	m.duration = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "dns_query_duration_seconds", Help: "DNS request duration.", Buckets: prometheus.DefBuckets})
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dns_upstream_requests_total", Help: "Upstream attempts including failures."}, []string{"upstream"})
	m.errors = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dns_upstream_errors_total", Help: "Failed upstream attempts."}, []string{"upstream"})
	m.blockedCounter = prometheus.NewCounter(prometheus.CounterOpts{Name: "dns_queries_blocked_total", Help: "Queries blocked by DNS policy."})
	m.overload = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dns_overload_total", Help: "DNS work rejected by bounded overload reason."}, []string{"reason"})
	m.tcpConnections = prometheus.NewGauge(prometheus.GaugeOpts{Name: "dns_tcp_connections", Help: "Currently accepted client TCP connections."})
	r.MustRegister(m.queries, m.duration, m.requests, m.errors, prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "dns_cache_entries", Help: "Live cache entries."}, func() float64 { return float64(c.Stats().Entries) }), prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "dns_cache_hits_total", Help: "Cache hits."}, func() float64 { return float64(c.Stats().Hits) }), prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "dns_cache_misses_total", Help: "Cache misses."}, func() float64 { return float64(c.Stats().Misses) }), m.blockedCounter, m.overload, m.tcpConnections)
	return m
}
func (m *Metrics) Overload(reason string) { m.overload.WithLabelValues(reason).Inc() }
func (m *Metrics) TCPConnection(open bool) {
	if open {
		m.tcpConnections.Inc()
	} else {
		m.tcpConnections.Dec()
	}
}
func (m *Metrics) Query(kind, source string, rcode int, elapsed time.Duration) {
	code, ok := wire.RcodeToString[rcode]
	if !ok {
		code = "other"
	}
	m.queries.WithLabelValues(kind, source, code).Inc()
	m.duration.Observe(elapsed.Seconds())
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total++
	if source == "blocked" {
		m.blocked++
		m.blockedCounter.Inc()
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
	m.requests.WithLabelValues(server).Inc()
	m.errors.WithLabelValues(server).Add(0)
	if failed {
		m.errors.WithLabelValues(server).Inc()
	}
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
