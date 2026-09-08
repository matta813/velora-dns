package api

import (
	"context"
	"encoding/json"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/querylog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQueriesRealStorageJSONAndValidation(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now()
	entry := querylog.Entry{OccurredAt: now, Domain: "example.test.", ClientIP: "127.0.0.1", Type: "A", Rcode: "NOERROR", Source: "upstream", Upstream: "192.0.2.1:53", Duration: 2 * time.Millisecond}
	if err = db.WriteQueries(ctx, []querylog.Entry{entry}, now.Add(-time.Hour), 100); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerQueries(mux, db, true)
	w := zoneRequest(mux, "GET", "/api/v1/queries?domain=EXAMPLE&type=A&client=127.0.0.1&source=upstream", "", "")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var body struct{ Data []map[string]any }
	if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 {
		t.Fatal(w.Body.String())
	}
	for _, key := range []string{"id", "occurred_at", "client_ip", "domain", "type", "rcode", "duration", "source", "upstream", "cache_hit"} {
		if _, ok := body.Data[0][key]; !ok {
			t.Fatalf("missing UI field %s: %s", key, w.Body.String())
		}
	}
	for _, q := range []string{"limit=0", "limit=501", "limit=no", "client=garbage", "type=garbage", "source=garbage", "before=-1"} {
		if got := zoneRequest(mux, "GET", "/api/v1/queries?"+q, "", ""); got.Code != 400 {
			t.Fatalf("%s: %d", q, got.Code)
		}
	}
	if w = zoneRequest(mux, "POST", "/api/v1/queries", "", ""); w.Code != 405 {
		t.Fatalf("method: %d", w.Code)
	}
	w = zoneRequest(mux, "GET", "/api/v1/query-stats?window=24h&limit=10", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"top_domains":[{"value":"example.test.","count":1}]`) {
		t.Fatalf("summary: %d %s", w.Code, w.Body.String())
	}
	for _, query := range []string{"window=forever", "limit=0", "limit=51"} {
		if got := zoneRequest(mux, "GET", "/api/v1/query-stats?"+query, "", ""); got.Code != 400 {
			t.Fatalf("summary %s: %d", query, got.Code)
		}
	}
	if got := zoneRequest(mux, "POST", "/api/v1/query-stats", "", ""); got.Code != 405 {
		t.Fatalf("summary method: %d", got.Code)
	}
}

func TestQueriesUnavailableWhenDisabled(t *testing.T) {
	mux := http.NewServeMux()
	registerQueries(mux, nil, false)
	for _, path := range []string{"/api/v1/queries", "/api/v1/query-stats"} {
		if got := zoneRequest(mux, "GET", path, "", ""); got.Code != 503 || !strings.Contains(got.Body.String(), "query_logging_disabled") {
			t.Fatalf("%s: %d %s", path, got.Code, got.Body.String())
		}
	}
}
