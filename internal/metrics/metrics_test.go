package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matta813/velora-dns/internal/cache"
)

func TestTransportResourceMetrics(t *testing.T) {
	m := New(cache.New(1))
	m.TCPConnection(true)
	m.Overload("client_rate")
	recorder := httptest.NewRecorder()
	m.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, metric := range []string{`dns_tcp_connections 1`, `dns_overload_total{reason="client_rate"} 1`} {
		if !strings.Contains(body, metric) {
			t.Fatalf("missing %q in metrics", metric)
		}
	}
	m.TCPConnection(false)
}
