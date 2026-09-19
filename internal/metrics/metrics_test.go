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
