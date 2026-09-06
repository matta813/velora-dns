package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/metrics"
)

type fakeDB struct{ err error }

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
		handler(tc.db, tc.ready).ServeHTTP(w, httptest.NewRequest("GET", "/ready", nil))
		if w.Code != tc.code {
			t.Fatalf("ready: %d", w.Code)
		}
	}
}
func TestAPIAndOriginProtection(t *testing.T) {
	h := handler(fakeDB{}, true)
	for _, path := range []string{"/health", "/api/v1/status", "/api/v1/version", "/api/v1/stats", "/api/v1/cache", "/api/v1/config", "/metrics"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	for _, tc := range []struct {
		method, path, origin, content string
		code                          int
	}{{"DELETE", "/api/v1/cache", "https://attacker.test", "application/json", 403}, {"DELETE", "/api/v1/cache", "", "", 415}, {"DELETE", "/api/v1/cache", "", "application/json", 200}, {"GET", "/api/v1/zones", "", "", 404}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
}
