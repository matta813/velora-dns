package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
)

func TestMetricsSnapshotAndPrometheusOutput(t *testing.T) {
	m := New(cache.New(10))
	m.Query("A", "forwarded", 0, 15*time.Millisecond)
	m.Query("AAAA", "blocked", 999, 20*time.Millisecond)
	m.Upstream("1.1.1.1:53", true)
	m.Overload("qps")

	snapshot := m.Snapshot()
	if snapshot.Queries != 2 || snapshot.Blocked != 1 || snapshot.QPS <= 0 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	recorder := httptest.NewRecorder()
	m.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d", recorder.Code)
	}
	for _, want := range []string{"dns_queries_total", `rcode="NOERROR"`, `rcode="other"`, "dns_upstream_errors_total", "dns_overload_rejections_total"} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("metrics output missing %q:\n%s", want, recorder.Body.String())
		}
	}
}

func TestStatisticsResetPreservesProcessTelemetry(t *testing.T) {
	m := New(cache.New(10))
	m.Query("A", "blocked", 0, time.Millisecond)
	m.Upstream("192.0.2.1:53", true)
	requests, errors := m.UpstreamCounters()
	if requests["192.0.2.1:53"] != 1 || errors["192.0.2.1:53"] != 1 {
		t.Fatalf("upstream counters: %v %v", requests, errors)
	}
	m.Reset()
	if got := m.Snapshot(); got.Queries != 0 || got.Blocked != 0 || got.QPS != 0 {
		t.Fatalf("reset snapshot: %+v", got)
	}
	requests, errors = m.UpstreamCounters()
	if len(requests) != 0 || len(errors) != 0 {
		t.Fatalf("upstream counters after reset: %v %v", requests, errors)
	}
	m.Restore(3, 1)
	m.RestoreUpstreamCounters(map[string]uint64{"192.0.2.1:53": 2}, map[string]uint64{"192.0.2.1:53": 1})
	if got := m.Snapshot(); got.Queries != 3 || got.Blocked != 1 || got.QPS != 0 {
		t.Fatalf("restored snapshot: %+v", got)
	}
	response := httptest.NewRecorder()
	m.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(response.Body.String(), `dns_upstream_requests_total{upstream="192.0.2.1:53"} 2`) {
		t.Fatalf("restored Prometheus counter missing: %s", response.Body.String())
	}
}
