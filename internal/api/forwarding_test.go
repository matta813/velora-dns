package api

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/forwarding"
	"github.com/matta813/velora-dns/internal/metrics"
)

type forwardMemory struct{ rules []forwarding.Rule }

func (m *forwardMemory) LoadForwardRules(context.Context) ([]forwarding.Rule, error) {
	return m.rules, nil
}
func (m *forwardMemory) SaveForwardRule(_ context.Context, rule forwarding.Rule) (forwarding.Rule, error) {
	if rule.ID == 0 {
		rule.ID = int64(len(m.rules) + 1)
		m.rules = append(m.rules, rule)
	}
	return rule, nil
}
func (m *forwardMemory) DeleteForwardRule(context.Context, int64) error { return nil }

func TestForwardingEndpoints(t *testing.T) {
	service, err := forwarding.NewService(context.Background(), &forwardMemory{}, forwarding.Options{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	c := cache.New(10)
	h := New(Dependencies{Database: fakeDB{}, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now(), Forwarding: service})
	send := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if w := send("POST", "/api/v1/forwarding", `{"domain":"corp.example","upstreams":["10.0.0.10"]}`); w.Code != 201 || !bytes.Contains(w.Body.Bytes(), []byte(`"10.0.0.10:53"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"enabled":true`)) {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := send("POST", "/api/v1/forwarding", `{"domain":"corp.example","upstreams":["10.0.0.11"]}`); w.Code != 409 {
		t.Fatalf("duplicate: %d", w.Code)
	}
	if w := send("POST", "/api/v1/forwarding", `{"domain":"bad domain","upstreams":["10.0.0.11"]}`); w.Code != 400 {
		t.Fatalf("invalid: %d", w.Code)
	}
	if w := send("GET", "/api/v1/forwarding", ""); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"health":[{"address":"10.0.0.10:53"`)) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := send("POST", "/api/v1/forwarding/1/test", `{"name":"other.example"}`); w.Code != 400 {
		t.Fatalf("test outside domain: %d", w.Code)
	}
	if w := send("POST", "/api/v1/forwarding/1/test", `{"type":"BOGUS"}`); w.Code != 400 {
		t.Fatalf("test bad type: %d", w.Code)
	}
	if w := send("PUT", "/api/v1/forwarding/7", `{"domain":"x.example","upstreams":["10.0.0.1"]}`); w.Code != 404 {
		t.Fatalf("update missing: %d", w.Code)
	}
	if w := send("GET", "/api/v1/status", ""); !bytes.Contains(w.Body.Bytes(), []byte("conditional_forwarding")) {
		t.Fatalf("capability missing: %s", w.Body.String())
	}
}
