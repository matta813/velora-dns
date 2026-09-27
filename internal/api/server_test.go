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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	internaldns "github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/querylog"
	"github.com/miekg/dns"
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

func TestUpstreamHealthEndpoint(t *testing.T) {
	c := cache.New(10)
	health := internaldns.NewUpstreamHealth([]string{"127.0.0.1:53"})
	h := New(Dependencies{Database: fakeDB{}, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now(), UpstreamHealth: health})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/upstreams/health", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"state":"unknown"`)) {
		t.Fatalf("upstream health: %d %s", w.Code, w.Body.String())
	}
}
func TestCacheEntriesEndpoint(t *testing.T) {
	c := cache.New(10)
	q := new(dns.Msg)
	q.SetQuestion("example.test.", dns.TypeA)
	answer := new(dns.Msg)
	answer.SetReply(q)
	answer.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "example.test.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: []byte{192, 0, 2, 1}}}
	c.Put(q, answer)
	h := New(Dependencies{Database: fakeDB{}, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now()})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/api/v1/cache/entries?limit=1", nil))
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"name":"example.test."`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"192.0.2.1"`)) {
		t.Fatalf("cache entries: status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/api/v1/cache/entries?limit=201", nil))
	if w.Code != 400 {
		t.Fatalf("invalid limit: %d", w.Code)
	}
}

func TestCacheEntriesFilterAndInvalidate(t *testing.T) {
	c := cache.New(10)
	for _, name := range []string{"example.test.", "www.example.test.", "other.test."} {
		q := new(dns.Msg)
		q.SetQuestion(name, dns.TypeA)
		answer := new(dns.Msg)
		answer.SetReply(q)
		answer.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: []byte{192, 0, 2, 1}}}
		c.Put(q, answer)
	}
	h := New(Dependencies{Database: fakeDB{}, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now()})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/api/v1/cache/entries?domain=example&type=a", nil))
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"total":2`)) {
		t.Fatalf("filtered entries: %d %s", w.Code, w.Body.String())
	}
	for _, query := range []string{"type=BOGUS", "domain=" + strings.Repeat("a", 254)} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/api/v1/cache/entries?"+query, nil))
		if w.Code != 400 {
			t.Fatalf("%s: expected 400, got %d", query, w.Code)
		}
	}
	invalidate := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "http://127.0.0.1/api/v1/cache/invalidate", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if w = invalidate(`{"name":"bad name!","type":"A"}`); w.Code != 400 {
		t.Fatalf("invalid name: %d", w.Code)
	}
	if w = invalidate(`{"name":"example.test","type":"TXT"}`); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"removed":0`)) {
		t.Fatalf("type mismatch: %d %s", w.Code, w.Body.String())
	}
	if w = invalidate(`{"name":"example.test","include_subdomains":true}`); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"removed":2`)) {
		t.Fatalf("domain invalidate: %d %s", w.Code, w.Body.String())
	}
	if c.Stats().Entries != 1 {
		t.Fatalf("expected one surviving entry, got %d", c.Stats().Entries)
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

func TestWebFallbackIncludesAllUIRoutes(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/updates", "/backup", "/dhcp", "/cluster"} {
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

func TestConfigApplyFailureRestoresPreviousConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	old := config.Default()
	if err := old.Save(path); err != nil {
		t.Fatal(err)
	}
	cache := cache.New(10)
	var applied []string
	h := New(Dependencies{
		Database: fakeDB{}, DNS: fakeDNS(true), Cache: cache, Metrics: metrics.New(cache),
		Config: old, ConfigPath: path, Started: time.Now(),
		ApplyConfig: func(candidate config.Config) error {
			applied = append(applied, candidate.LogLevel)
			if candidate.LogLevel == "debug" {
				return errors.New("cannot apply debug")
			}
			return nil
		},
	})
	request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1/api/v1/config", bytes.NewBufferString(`{"log_level":"debug"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || !bytes.Contains(response.Body.Bytes(), []byte(`config_apply_failed`)) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
	if len(applied) != 2 || applied[0] != "debug" || applied[1] != old.LogLevel {
		t.Fatalf("runtime rollback was not applied: %v", applied)
	}
	persisted, err := os.ReadFile(path)
	if err != nil || bytes.Contains(persisted, []byte("debug")) {
		t.Fatalf("failed candidate reached disk: %s (%v)", persisted, err)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/config", nil))
	if !bytes.Contains(response.Body.Bytes(), []byte(`"log_level":"info"`)) {
		t.Fatalf("failed candidate reached API: %s", response.Body.String())
	}
}

func TestConfigSaveFailureRollsBackRuntime(t *testing.T) {
	cache := cache.New(10)
	old := config.Default()
	var applied []string
	h := New(Dependencies{
		Database: fakeDB{}, DNS: fakeDNS(true), Cache: cache, Metrics: metrics.New(cache),
		Config: old, ConfigPath: filepath.Join(t.TempDir(), "missing", "config.yaml"), Started: time.Now(),
		ApplyConfig: func(candidate config.Config) error {
			applied = append(applied, candidate.LogLevel)
			return nil
		},
	})
	request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1/api/v1/config", bytes.NewBufferString(`{"log_level":"debug"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || !bytes.Contains(response.Body.Bytes(), []byte(`config_save_failed`)) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
	if len(applied) != 2 || applied[0] != "debug" || applied[1] != old.LogLevel {
		t.Fatalf("runtime rollback was not applied: %v", applied)
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

func configRequest(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://127.0.0.1"+path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestConfigErrorsNameFieldsAndDryRunChangesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	old := config.Default()
	if err := old.Save(path); err != nil {
		t.Fatal(err)
	}
	c := cache.New(10)
	applied := 0
	h := New(Dependencies{
		Database: fakeDB{}, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: old, ConfigPath: path, Started: time.Now(),
		ApplyConfig: func(candidate config.Config) error {
			applied++
			if fields := config.RestartRequiredFields(old, candidate); len(fields) > 0 {
				return &config.RestartRequiredError{Fields: fields}
			}
			return nil
		},
	})
	bad := `{"dns":{"allowed_clients":["10.0.0.0/8","nope"],"retries":7}}`
	response := configRequest(h, http.MethodPut, "/api/v1/config", bad)
	var body struct {
		Error struct {
			Code   string
			Fields []config.FieldError
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != 400 || body.Error.Code != "invalid_config" || len(body.Error.Fields) != 2 || body.Error.Fields[0].Field != "dns.allowed_clients[1]" {
		t.Fatalf("invalid: %d %s", response.Code, response.Body.String())
	}
	if applied != 0 {
		t.Fatal("invalid config must not be applied")
	}
	response = configRequest(h, http.MethodPut, "/api/v1/config", `{"dns":{"listen":["127.0.0.1:5300"]}}`)
	if response.Code != 409 || !bytes.Contains(response.Body.Bytes(), []byte(`"field":"dns.listen"`)) {
		t.Fatalf("restart required: %d %s", response.Code, response.Body.String())
	}

	var check struct{ Data configCheck }
	response = configRequest(h, http.MethodPost, "/api/v1/config/validate", bad)
	if err := json.Unmarshal(response.Body.Bytes(), &check); err != nil || response.Code != 200 || check.Data.Valid || len(check.Data.Errors) != 2 {
		t.Fatalf("dry run invalid: %d %s", response.Code, response.Body.String())
	}
	response = configRequest(h, http.MethodPost, "/api/v1/config/validate", `{"dns":{"listen":["127.0.0.1:5300"]},"filtering":{"block_mode":"ZERO"}}`)
	if err := json.Unmarshal(response.Body.Bytes(), &check); err != nil || check.Data.Valid || len(check.Data.RestartRequired) != 1 || check.Data.RestartRequired[0] != "dns.listen" {
		t.Fatalf("dry run restart: %s", response.Body.String())
	}
	response = configRequest(h, http.MethodPost, "/api/v1/config/validate", `{"filtering":{"block_mode":"ZERO"}}`)
	if err := json.Unmarshal(response.Body.Bytes(), &check); err != nil || !check.Data.Valid {
		t.Fatalf("dry run valid: %s", response.Body.String())
	}
	if applied != 1 {
		t.Fatalf("dry runs must not apply anything: %d", applied)
	}
	persisted, _ := os.ReadFile(path)
	if bytes.Contains(persisted, []byte("ZERO")) || bytes.Contains(persisted, []byte("5300")) {
		t.Fatalf("dry run reached disk: %s", persisted)
	}
}

func TestConcurrentConfigChangesAreSerialized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	old := config.Default()
	c := cache.New(10)
	var active, maxActive atomic.Int32
	h := New(Dependencies{
		Database: fakeDB{}, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: old, ConfigPath: path, Started: time.Now(),
		ApplyConfig: func(config.Config) error {
			now := active.Add(1)
			for {
				seen := maxActive.Load()
				if now <= seen || maxActive.CompareAndSwap(seen, now) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			active.Add(-1)
			return nil
		},
	})
	levels := []string{"debug", "warn", "error", "info"}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		level := levels[i%len(levels)]
		wg.Go(func() {
			if response := configRequest(h, http.MethodPut, "/api/v1/config", `{"log_level":"`+level+`"}`); response.Code != 200 {
				t.Errorf("%s: %d %s", level, response.Code, response.Body.String())
			}
		})
	}
	wg.Wait()
	if maxActive.Load() != 1 {
		t.Fatalf("configuration applies overlapped: %d", maxActive.Load())
	}
	// Memory and disk agree on the last writer.
	response := configRequest(h, http.MethodGet, "/api/v1/config", "")
	saved, err := config.Load(path)
	if err != nil || !bytes.Contains(response.Body.Bytes(), []byte(`"log_level":"`+saved.LogLevel+`"`)) {
		t.Fatalf("memory %s vs disk %s (%v)", response.Body.String(), saved.LogLevel, err)
	}
}

func TestConfigRollbackFailureAndReadinessRollbackAreReported(t *testing.T) {
	c := cache.New(10)
	old := config.Default()
	var events []string
	notify := func(event database.SystemEventInput) { events = append(events, event.Severity) }
	// Apply and rollback both fail.
	h := New(Dependencies{
		Database: fakeDB{}, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: old, ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), Started: time.Now(), NotifyEvent: notify,
		ApplyConfig: func(config.Config) error { return errors.New("listener refused") },
	})
	response := configRequest(h, http.MethodPut, "/api/v1/config", `{"filtering":{"block_mode":"ZERO"}}`)
	if response.Code != 500 || !bytes.Contains(response.Body.Bytes(), []byte("config_rollback_failed")) || len(events) != 1 || events[0] != "critical" {
		t.Fatalf("rollback failure: %d %s %v", response.Code, response.Body.String(), events)
	}
	// Services not ready after apply: the previous settings are restored.
	events = nil
	var applied []string
	h = New(Dependencies{
		Database: fakeDB{}, DNS: fakeDNS(false), Cache: c, Metrics: metrics.New(c), Config: old, ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), Started: time.Now(), NotifyEvent: notify,
		ApplyConfig: func(candidate config.Config) error {
			applied = append(applied, candidate.Filtering.BlockMode)
			return nil
		},
	})
	response = configRequest(h, http.MethodPut, "/api/v1/config", `{"filtering":{"block_mode":"ZERO"}}`)
	if response.Code != 503 || !bytes.Contains(response.Body.Bytes(), []byte("config_not_ready")) || len(applied) != 2 || applied[1] != old.Filtering.BlockMode || len(events) != 1 || events[0] != "warning" {
		t.Fatalf("readiness rollback: %d %s %v %v", response.Code, response.Body.String(), applied, events)
	}
}
