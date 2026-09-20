package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/querylog"
)

type fakeDB struct{ err error }

type fakeQueryStore struct{}

func (fakeQueryStore) ListQueries(context.Context, querylog.Filter) ([]querylog.Entry, error) {
	return nil, nil
}
func (fakeQueryStore) QuerySummary(context.Context, time.Time, time.Time, int) (querylog.Summary, error) {
	return querylog.Summary{}, nil
}

func (d fakeDB) Ping(context.Context) error { return d.err }

type fakeDNS bool

func (d fakeDNS) Ready() bool         { return bool(d) }
func (d fakeDNS) Addresses() []string { return []string{"127.0.0.1:5353"} }
func handler(db Database, ready bool) http.Handler {
	c := cache.New(10)
	return New(Dependencies{Database: db, DNS: fakeDNS(ready), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now()})
}
func TestReadiness(t *testing.T) {
	for _, tc := range []struct {
		db    Database
		ready bool
		code  int
	}{{fakeDB{}, true, 200}, {fakeDB{errors.New("offline")}, true, 503}, {fakeDB{}, false, 503}} {
		w := httptest.NewRecorder()
		handler(tc.db, tc.ready).ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/ready", nil))
		if w.Code != tc.code {
			t.Fatalf("ready: %d", w.Code)
		}
	}
}
func TestAPIAndOriginProtection(t *testing.T) {
	h := handler(fakeDB{}, true)
	for _, path := range []string{"/health", "/api/v1/status", "/api/v1/version", "/api/v1/stats", "/api/v1/cache", "/api/v1/config", "/metrics"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1"+path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	for _, tc := range []struct {
		method, path, origin, content string
		code                          int
	}{{"DELETE", "/api/v1/cache", "https://attacker.test", "application/json", 403}, {"DELETE", "/api/v1/cache", "", "", 415}, {"DELETE", "/api/v1/cache", "", "application/json", 200}, {"GET", "/api/v1/zones", "", "", 404}} {
		r := httptest.NewRequest(tc.method, "http://127.0.0.1"+tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
}

func TestHostAndRequestLimits(t *testing.T) {
	h := handler(fakeDB{}, true)
	for _, tc := range []struct {
		method, host, path string
		length             int64
		code               int
	}{
		{"GET", "attacker.test", "/api/v1/config", 0, 403},
		{"GET", "127.0.0.1:8080", "/api/v1/config", 0, 200},
		{"POST", "127.0.0.1:8080", "/api/v1/config", 0, 405},
		{"PUT", "127.0.0.1:8080", "/api/v1/config", 0, 500},
		{"POST", "127.0.0.1", "/api/v1/status", 0, 405},
		{"DELETE", "127.0.0.1", "/api/v1/cache", 2 << 20, 413},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, nil)
		r.ContentLength = tc.length
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s %s: got %d, want %d", tc.host, tc.path, w.Code, tc.code)
		}
	}
}

func TestWildcardHostAllowsRemoteWebUI(t *testing.T) {
	c := cache.New(10)
	cfg := config.Default()
	cfg.HTTP.AllowedHosts = []string{"*"}
	h := New(Dependencies{Database: fakeDB{}, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: cfg, Started: time.Now()})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://remote.example/api/v1/status", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("wildcard host status = %d", w.Code)
	}
}

func TestWebFallbackIncludesUpdateAndBackupRoutes(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/updates", "/backup"} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
		w := httptest.NewRecorder()
		web(func() string { return tmpDir }).ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("route %s returned %d, want 200", path, w.Code)
		}
	}
}

func TestConfigSaveReenablesQueryLoggingWithoutRestart(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	c := cache.New(10)
	cfg := config.Default()
	h := New(Dependencies{
		Database:   fakeDB{},
		DNS:        fakeDNS(true),
		Cache:      c,
		Metrics:    metrics.New(c),
		Config:     cfg,
		ConfigPath: tmpFile,
		Queries:    fakeQueryStore{},
		Started:    time.Now(),
	})

	payload := []byte(`{"query_log":{"enabled":true}}`)
	putReq := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:8080/api/v1/config", bytes.NewReader(payload))
	putReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, putReq)
	if w.Code != http.StatusOK {
		t.Fatalf("put config failed: got %d (%s), want 200", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/queries", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("query endpoint did not re-enable live: got %d, want 200", w.Code)
	}
}

func TestConfigSavePreservesDatabasePathAndUpdatesInMemory(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "config.yaml")
	c := cache.New(10)
	cfg := config.Default()
	cfg.DatabasePath = "/var/lib/velora/velora.db"
	h := New(Dependencies{
		Database:   fakeDB{},
		DNS:        fakeDNS(true),
		Cache:      c,
		Metrics:    metrics.New(c),
		Config:     cfg,
		ConfigPath: tmpFile,
		Started:    time.Now(),
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("initial get: got %d, want 200", w.Code)
	}

	// Web UI sends JSON without database_path (since database_path is json:"-")
	payload := []byte(`{
		"dns": {
			"listen": ["0.0.0.0:53"],
			"upstreams": ["1.1.1.1:53", "9.9.9.9:53"],
			"allowed_clients": ["192.168.20.0/26"]
		},
		"http": {
			"listen": "0.0.0.0:8080",
			"allowed_hosts": ["*"]
		},
		"query_log": {
			"enabled": true
		},
		"log_level": "debug"
	}`)

	putReq := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:8080/api/v1/config", bytes.NewReader(payload))
	putReq.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, putReq)
	if w.Code != http.StatusOK {
		t.Fatalf("put config failed: got %d (%s), want 200", w.Code, w.Body.String())
	}

	// Verify GET reflects the in-memory saved changes
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/v1/config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("get updated config: got %d, want 200", w.Code)
	}
	var res struct {
		Data config.Config `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.Data.LogLevel != "debug" {
		t.Fatalf("in-memory config not updated: got %s, want debug", res.Data.LogLevel)
	}
	if len(res.Data.DNS.Listen) != 1 || res.Data.DNS.Listen[0] != "0.0.0.0:53" {
		t.Fatalf("dns.listen not updated: got %v", res.Data.DNS.Listen)
	}
	if len(res.Data.DNS.AllowedClients) != 1 || res.Data.DNS.AllowedClients[0] != "192.168.20.0/26" {
		t.Fatalf("dns.allowed_clients not updated: got %v", res.Data.DNS.AllowedClients)
	}

	// Verify saved file on disk preserved DatabasePath
	loaded, err := config.Load(tmpFile)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}
	if loaded.DatabasePath != "/var/lib/velora/velora.db" {
		t.Fatalf("database_path was overwritten or lost: got %q", loaded.DatabasePath)
	}
}
