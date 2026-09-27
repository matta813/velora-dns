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
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/rewrites"
)

type rewriteMemory struct{ rules []rewrites.Rule }

func (m *rewriteMemory) LoadRewrites(context.Context) ([]rewrites.Rule, error) { return m.rules, nil }
func (m *rewriteMemory) SaveRewrite(_ context.Context, rule rewrites.Rule) (rewrites.Rule, error) {
	if rule.ID == 0 {
		rule.ID = int64(len(m.rules) + 1)
		m.rules = append(m.rules, rule)
	}
	return rule, nil
}
func (m *rewriteMemory) DeleteRewrite(context.Context, int64) error { return nil }

func TestRewriteEndpoints(t *testing.T) {
	service, err := rewrites.NewService(context.Background(), &rewriteMemory{}, rewrites.Hints{Blocked: func(name string) bool { return name == "ads.home" }})
	if err != nil {
		t.Fatal(err)
	}
	c := cache.New(10)
	h := New(Dependencies{Database: fakeDB{}, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now(), Rewrites: service})
	send := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if w := send("POST", "/api/v1/rewrites", `{"name":"nas.home","type":"A","value":"192.0.2.5"}`); w.Code != 201 || !bytes.Contains(w.Body.Bytes(), []byte(`"enabled":true`)) {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := send("POST", "/api/v1/rewrites", `{"name":"nas.home","type":"CNAME","value":"other.home"}`); w.Code != 409 || !bytes.Contains(w.Body.Bytes(), []byte("rewrite_conflict")) {
		t.Fatalf("conflict: %d %s", w.Code, w.Body.String())
	}
	if w := send("POST", "/api/v1/rewrites", `{"name":"nas.home","type":"A","value":"not-an-ip"}`); w.Code != 400 {
		t.Fatalf("invalid: %d", w.Code)
	}
	if w := send("POST", "/api/v1/rewrites", `{"name":"ads.home","type":"A","value":"192.0.2.6"}`); w.Code != 201 {
		t.Fatalf("create blocked: %d", w.Code)
	}
	if w := send("GET", "/api/v1/rewrites", ""); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"blocked_by":"blocklist"`)) {
		t.Fatalf("list hints: %d %s", w.Code, w.Body.String())
	}
	if w := send("DELETE", "/api/v1/rewrites/9", ""); w.Code != 404 {
		t.Fatalf("delete missing: %d", w.Code)
	}
}
