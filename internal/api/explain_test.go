package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/querylog"
	"github.com/matta813/velora-dns/internal/rewrites"
	wire "github.com/miekg/dns"
)

type explainUpstream struct{ calls atomic.Int32 }

func (u *explainUpstream) Resolve(_ context.Context, q *wire.Msg) (*wire.Msg, string, error) {
	u.calls.Add(1)
	return new(wire.Msg).SetReply(q), "fake", nil
}

type explainEnv struct {
	h        http.Handler
	db       *database.Store
	cache    *cache.Cache
	metrics  *metrics.Metrics
	upstream *explainUpstream
}

func newExplainEnv(t *testing.T) explainEnv {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "explain.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, u := range []struct{ name, role string }{{"admin", "admin"}, {"viewer", "viewer"}} {
		if _, err = db.CreateUser(ctx, u.name, u.name+" password long", u.role); err != nil {
			t.Fatal(err)
		}
	}
	rules, err := rewrites.NewService(ctx, db, rewrites.Hints{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rules.Create(ctx, rewrites.Rule{Name: "nas.home", Type: "A", Value: "192.0.2.5", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	memory := cache.New(10)
	q := new(wire.Msg)
	q.SetQuestion("cached.example.", wire.TypeA)
	q.RecursionDesired = true
	cached := new(wire.Msg).SetReply(q)
	rr, _ := wire.NewRR("cached.example. 300 IN A 198.51.100.4")
	cached.Answer = []wire.RR{rr}
	memory.Put(q, cached)
	upstream := &explainUpstream{}
	observer := metrics.New(memory)
	resolver := dns.NewResolver(&rewrites.Local{Rewrites: rules}, nil, memory, upstream, "NXDOMAIN")
	h := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: memory, Resolver: resolver, Metrics: observer, Config: config.Default(), Started: time.Now()})
	return explainEnv{h: h, db: db, cache: memory, metrics: observer, upstream: upstream}
}

func explainPost(h http.Handler, body, contentType string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/v1/diagnostics/explain", strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if mutate != nil {
		mutate(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestExplainEndpointRolesAndNoSideEffects(t *testing.T) {
	env := newExplainEnv(t)
	viewer, viewerCSRF := loginForTest(t, env.h, "viewer", "viewer password long")
	as := func(cookie *http.Cookie, csrf string) func(*http.Request) {
		return func(r *http.Request) {
			r.AddCookie(cookie)
			if csrf != "" {
				r.Header.Set("X-CSRF-Token", csrf)
			}
		}
	}

	auditBefore, _ := env.db.ListAudit(context.Background(), database.AuditFilter{Limit: 100})
	cacheBefore, metricsBefore := env.cache.Stats(), env.metrics.Snapshot()
	logBefore, _ := env.db.ListQueries(context.Background(), querylog.Filter{Limit: 100})

	if w := explainPost(env.h, `{"name":"nas.home"}`, "application/json", nil); w.Code != 401 {
		t.Fatalf("no credentials: %d %s", w.Code, w.Body.String())
	}
	if w := explainPost(env.h, `{"name":"nas.home"}`, "application/json", as(viewer, "")); w.Code != 403 || !strings.Contains(w.Body.String(), "invalid_csrf") {
		t.Fatalf("viewer without CSRF: %d %s", w.Code, w.Body.String())
	}
	w := explainPost(env.h, `{"name":"NAS.home.","type":"a","client":"192.168.1.9"}`, "application/json", as(viewer, viewerCSRF))
	if w.Code != 200 {
		t.Fatalf("viewer explain: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Data dns.Explanation `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got := body.Data; got.Source != "local" || got.Winner.Stage != "rewrite" || got.Winner.Rules[0].Name != "nas.home" || got.Client != "192.168.1.9" || len(got.Answers) != 1 {
		t.Fatalf("explanation: %+v", got)
	}
	w = explainPost(env.h, `{"name":"cached.example","type":"A"}`, "application/json", as(viewer, viewerCSRF))
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data.Winner.Stage != "cache" || body.Data.Winner.TTL == nil || *body.Data.Winner.TTL > 300 || *body.Data.Winner.TTL < 290 {
		t.Fatalf("cached explanation: %d %s", w.Code, w.Body.String())
	}
	w = explainPost(env.h, `{"name":"nowhere.example"}`, "application/json", as(viewer, viewerCSRF))
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data.Winner.Result != "would_forward" || len(body.Data.Answers) != 0 {
		t.Fatalf("upstream explanation: %d %s", w.Code, w.Body.String())
	}

	// Other writes stay closed to viewers.
	if w = authRequest(env.h, "DELETE", "/api/v1/cache", "", viewer, viewerCSRF); w.Code != 403 {
		t.Fatalf("viewer cache flush: %d", w.Code)
	}
	// No side effects: no upstream call, no cache or metric change, no query
	// log entry and no audit entry for the explain calls.
	if env.upstream.calls.Load() != 0 {
		t.Fatal("explain contacted the upstream")
	}
	if env.cache.Stats() != cacheBefore {
		t.Fatalf("cache changed: %+v -> %+v", cacheBefore, env.cache.Stats())
	}
	if after := env.metrics.Snapshot(); after != metricsBefore {
		t.Fatalf("metrics changed: %+v -> %+v", metricsBefore, after)
	}
	if logAfter, _ := env.db.ListQueries(context.Background(), querylog.Filter{Limit: 100}); len(logAfter) != len(logBefore) {
		t.Fatal("query log changed")
	}
	auditAfter, _ := env.db.ListAudit(context.Background(), database.AuditFilter{Limit: 100})
	for _, event := range auditAfter[:len(auditAfter)-len(auditBefore)] {
		if strings.Contains(event.Action+event.Target, "diagnostics/explain") {
			t.Fatalf("explain was audited: %+v", event)
		}
	}
}

func TestExplainEndpointReadTokenAllowed(t *testing.T) {
	env := newExplainEnv(t)
	admin, csrf := loginForTest(t, env.h, "admin", "admin password long")
	created := authRequest(env.h, "POST", "/api/v1/tokens", `{"name":"explain","scopes":["read"],"expires_in_hours":1}`, admin, csrf)
	var token struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &token); err != nil || created.Code != 201 {
		t.Fatalf("token: %d %s", created.Code, created.Body.String())
	}
	bearer := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token.Data.Token) }
	if w := explainPost(env.h, `{"name":"nas.home"}`, "application/json", bearer); w.Code != 200 {
		t.Fatalf("read token explain: %d %s", w.Code, w.Body.String())
	}
	if w := explainPost(env.h, `{"name":"nas.home"}`, "application/json", func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }); w.Code != 401 {
		t.Fatalf("bad token: %d", w.Code)
	}
	if w := bearerRequest(env.h, "DELETE", "/api/v1/cache", token.Data.Token); w.Code != 403 {
		t.Fatalf("read token flushed cache: %d", w.Code)
	}
}

func TestExplainEndpointBoundsInput(t *testing.T) {
	env := newExplainEnv(t)
	cookie, csrf := loginForTest(t, env.h, "viewer", "viewer password long")
	as := func(r *http.Request) { r.AddCookie(cookie); r.Header.Set("X-CSRF-Token", csrf) }
	label := strings.Repeat("a", 63)
	longest := label + "." + label + "." + label + "." + strings.Repeat("b", 61) // 253 characters
	tooLong := longest + "c"
	if len(longest) != 253 {
		t.Fatalf("test name has %d characters", len(longest))
	}
	for _, tc := range []struct {
		label, body, contentType string
		code                     int
		errCode                  string
	}{
		{"longest valid name", `{"name":"` + longest + `"}`, "application/json", 200, ""},
		{"name too long", `{"name":"` + tooLong + `"}`, "application/json", 400, "invalid_name"},
		{"label too long", `{"name":"` + strings.Repeat("a", 64) + `.example"}`, "application/json", 400, "invalid_name"},
		{"empty name", `{"name":""}`, "application/json", 400, "invalid_name"},
		{"missing name", `{}`, "application/json", 400, "invalid_name"},
		{"spaces in name", `{"name":"a b.example"}`, "application/json", 400, "invalid_name"},
		{"unknown type", `{"name":"a.example","type":"BOGUS"}`, "application/json", 400, "invalid_type"},
		{"unserved type", `{"name":"a.example","type":"AXFR"}`, "application/json", 400, "invalid_type"},
		{"bad client", `{"name":"a.example","client":"300.1.1.1"}`, "application/json", 400, "invalid_client"},
		{"hostname as client", `{"name":"a.example","client":"laptop.lan"}`, "application/json", 400, "invalid_client"},
		{"zoned client", `{"name":"a.example","client":"fe80::1%eth0"}`, "application/json", 400, "invalid_client"},
		{"ipv6 client", `{"name":"a.example","type":"AAAA","client":"2001:db8::1"}`, "application/json", 200, ""},
		{"invalid JSON", `{"name":`, "application/json", 400, "invalid_json"},
		{"unknown field", `{"name":"a.example","upstream":"1.1.1.1"}`, "application/json", 400, "invalid_json"},
		{"two documents", `{"name":"a.example"}{"name":"b.example"}`, "application/json", 400, "invalid_json"},
		{"wrong field type", `{"name":5}`, "application/json", 400, "invalid_json"},
		{"form content type", `name=a.example`, "application/x-www-form-urlencoded", 415, "unsupported_media_type"},
		{"missing content type", `{"name":"a.example"}`, "", 415, "unsupported_media_type"},
		{"body over 1 MiB", `{"name":"` + strings.Repeat("a", 2<<20) + `"}`, "application/json", 413, "payload_too_large"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			w := explainPost(env.h, tc.body, tc.contentType, as)
			if w.Code != tc.code || (tc.errCode != "" && !strings.Contains(w.Body.String(), `"code":"`+tc.errCode+`"`)) {
				t.Fatalf("status %d: %.200s", w.Code, w.Body.String())
			}
		})
	}
	if w := authRequest(env.h, "GET", "/api/v1/diagnostics/explain", "", cookie, ""); w.Code == 200 {
		t.Fatalf("GET explain served: %d", w.Code)
	}
	if env.upstream.calls.Load() != 0 {
		t.Fatal("explain contacted the upstream")
	}
}
