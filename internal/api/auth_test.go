package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/metrics"
)

func TestManagementAuthenticationCSRFAndRoles(t *testing.T) {
	db, err := database.Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.CreateUser(context.Background(), "admin", "admin password long", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateUser(context.Background(), "viewer", "viewer password long", "viewer"); err != nil {
		t.Fatal(err)
	}
	c := cache.New(10)
	h := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now()})
	if w := authRequest(h, "GET", "/api/v1/status", "", nil, ""); w.Code != 401 {
		t.Fatalf("unauthenticated: %d", w.Code)
	}
	if w := authRequest(h, "GET", "/api/v1/diagnostics", "", nil, ""); w.Code != 401 {
		t.Fatalf("unauthenticated diagnostics: %d", w.Code)
	}
	adminCookie, adminCSRF := loginForTest(t, h, "admin", "admin password long")
	if w := authRequest(h, "DELETE", "/api/v1/cache", "", adminCookie, ""); w.Code != 403 {
		t.Fatalf("missing csrf: %d", w.Code)
	}
	if w := authRequest(h, "POST", "/api/v1/users", `{"username":"operator","password":"operator password long","role":"operator"}`, adminCookie, adminCSRF); w.Code != 201 {
		t.Fatalf("create user: %d %s", w.Code, w.Body.String())
	}
	tokenResponse := authRequest(h, "POST", "/api/v1/tokens", `{"name":"monitor","scopes":["read"],"expires_in_hours":1}`, adminCookie, adminCSRF)
	if tokenResponse.Code != 201 {
		t.Fatalf("create token: %d %s", tokenResponse.Code, tokenResponse.Body.String())
	}
	var tokenBody struct {
		Data struct {
			ID    int64  `json:"id"`
			Token string `json:"token"`
		} `json:"data"`
	}
	if err = json.Unmarshal(tokenResponse.Body.Bytes(), &tokenBody); err != nil {
		t.Fatal(err)
	}
	if w := bearerRequest(h, "GET", "/api/v1/status", tokenBody.Data.Token); w.Code != 200 {
		t.Fatalf("token read: %d", w.Code)
	}
	if w := bearerRequest(h, "DELETE", "/api/v1/cache", tokenBody.Data.Token); w.Code != 403 {
		t.Fatalf("read token mutated: %d", w.Code)
	}
	if w := authRequest(h, "DELETE", "/api/v1/tokens/"+strconv.FormatInt(tokenBody.Data.ID, 10), "", adminCookie, adminCSRF); w.Code != 200 {
		t.Fatalf("revoke token: %d", w.Code)
	}
	if w := bearerRequest(h, "GET", "/api/v1/status", tokenBody.Data.Token); w.Code != 401 {
		t.Fatalf("revoked token accepted: %d", w.Code)
	}
	viewerCookie, viewerCSRF := loginForTest(t, h, "viewer", "viewer password long")
	if w := authRequest(h, "GET", "/api/v1/diagnostics", "", viewerCookie, ""); w.Code != 200 {
		t.Fatalf("viewer diagnostics: %d %s", w.Code, w.Body.String())
	}
	if w := authRequest(h, "DELETE", "/api/v1/cache", "", viewerCookie, viewerCSRF); w.Code != 403 {
		t.Fatalf("viewer mutation: %d", w.Code)
	}
}

func bearerRequest(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestLanguagePreferencesEndpoint(t *testing.T) {
	db, err := database.Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.CreateUser(context.Background(), "admin", "admin password long", "admin"); err != nil {
		t.Fatal(err)
	}
	c := cache.New(10)
	h := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now()})
	cookie, csrf := loginForTest(t, h, "admin", "admin password long")
	var w *httptest.ResponseRecorder
	w = authRequest(h, "GET", "/api/v1/preferences", "", cookie, "")
	if w.Code != 200 {
		t.Fatalf("get preferences: %d %s", w.Code, w.Body.String())
	}
	var got struct {
		Data struct {
			Language string `json:"language"`
			Theme    string `json:"theme"`
		} `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Data.Language != "en" || got.Data.Theme != "auto" {
		t.Fatalf("default preferences: %v %s", got.Data.Language, w.Body.String())
	}
	w = authRequest(h, "PUT", "/api/v1/preferences", `{"language":"de"}`, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("set preferences: %d %s", w.Code, w.Body.String())
	}
	w = authRequest(h, "PUT", "/api/v1/preferences", `{"language":"fr"}`, cookie, csrf)
	if w.Code != 400 {
		t.Fatalf("unsupported language: %d %s", w.Code, w.Body.String())
	}
	w = authRequest(h, "PUT", "/api/v1/preferences", `{"theme":"dark"}`, cookie, csrf)
	if w.Code != 200 {
		t.Fatalf("set theme: %d %s", w.Code, w.Body.String())
	}
	w = authRequest(h, "PUT", "/api/v1/preferences", `{"theme":"sepia"}`, cookie, csrf)
	if w.Code != 400 {
		t.Fatalf("unsupported theme: %d %s", w.Code, w.Body.String())
	}
	w = authRequest(h, "GET", "/api/v1/preferences", "", cookie, "")
	if w.Code != 200 {
		t.Fatalf("get preferences after update: %d %s", w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Data.Language != "de" || got.Data.Theme != "dark" {
		t.Fatalf("updated preferences: %v %s", got.Data.Language, w.Body.String())
	}
}

func TestRateLimitSettingsEndpoint(t *testing.T) {
	db, err := database.Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.CreateUser(context.Background(), "admin", "admin password long", "admin"); err != nil {
		t.Fatal(err)
	}
	c := cache.New(10)
	state := dns.NewRateLimitState(false, 1000, 100, 100)
	h := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now(), Settings: db, RateLimit: state})
	cookie, csrf := loginForTest(t, h, "admin", "admin password long")
	if w := authRequest(h, "GET", "/api/v1/settings/rate-limit", "", cookie, ""); w.Code != 200 {
		t.Fatalf("get rate limit: %d %s", w.Code, w.Body.String())
	}
	if w := authRequest(h, "PUT", "/api/v1/settings/rate-limit", `{"enabled":true,"global_qps":500,"client_qps":50,"rate_limit_burst":25}`, cookie, csrf); w.Code != 200 {
		t.Fatalf("set rate limit: %d %s", w.Code, w.Body.String())
	}
	if !state.Enabled() {
		t.Fatal("rate limit state not enabled after update")
	}
	client := netip.MustParseAddr("192.0.2.1")
	now := time.Now()
	for i := 0; i < 26; i++ {
		state.Allow(client, now)
	}
	if w := authRequest(h, "GET", "/api/v1/settings/rate-limit/status", "", nil, ""); w.Code != 401 {
		t.Fatalf("unauthenticated status: %d", w.Code)
	}
	if w := authRequest(h, "GET", "/api/v1/settings/rate-limit/status", "", cookie, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"rejected_total":1`) || strings.Contains(w.Body.String(), "192.0.2.1") {
		t.Fatalf("rate limit status: %d %s", w.Code, w.Body.String())
	}
	if w := authRequest(h, "PUT", "/api/v1/settings/rate-limit", `{"enabled":true,"global_qps":0,"client_qps":50,"rate_limit_burst":25}`, cookie, csrf); w.Code != 400 {
		t.Fatalf("invalid rate limit accepted: %d", w.Code)
	}
}

func TestUserManagement(t *testing.T) {
	db, err := database.Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.CreateUser(context.Background(), "admin", "admin password long", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateUser(context.Background(), "viewer", "viewer password long", "viewer"); err != nil {
		t.Fatal(err)
	}
	c := cache.New(10)
	h := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now()})
	adminCookie, adminCSRF := loginForTest(t, h, "admin", "admin password long")

	// List users
	w := authRequest(h, "GET", "/api/v1/users", "", adminCookie, "")
	if w.Code != 200 {
		t.Fatalf("list users: %d", w.Code)
	}

	// Create user
	w = authRequest(h, "POST", "/api/v1/users", `{"username":"operator","password":"operator password long","role":"operator"}`, adminCookie, adminCSRF)
	if w.Code != 201 {
		t.Fatalf("create user: %d %s", w.Code, w.Body.String())
	}

	// Get user ID from response
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	// Change role
	w = authRequest(h, "PUT", "/api/v1/users/"+strconv.FormatInt(created.Data.ID, 10)+"/role", `{"role":"viewer"}`, adminCookie, adminCSRF)
	if w.Code != 200 {
		t.Fatalf("change role: %d %s", w.Code, w.Body.String())
	}

	// Change password
	w = authRequest(h, "PUT", "/api/v1/users/"+strconv.FormatInt(created.Data.ID, 10)+"/password", `{"password":"new password long enough"}`, adminCookie, adminCSRF)
	if w.Code != 200 {
		t.Fatalf("change password: %d %s", w.Code, w.Body.String())
	}

	// Disable user
	w = authRequest(h, "DELETE", "/api/v1/users/"+strconv.FormatInt(created.Data.ID, 10), "", adminCookie, adminCSRF)
	if w.Code != 200 {
		t.Fatalf("disable user: %d %s", w.Code, w.Body.String())
	}

	// Cannot disable self
	adminID := int64(1) // admin user
	w = authRequest(h, "DELETE", "/api/v1/users/"+strconv.FormatInt(adminID, 10), "", adminCookie, adminCSRF)
	if w.Code != 400 {
		t.Fatalf("admin disable self: expected 400, got %d", w.Code)
	}

	// Viewer cannot manage users
	viewerCookie, viewerCSRF := loginForTest(t, h, "viewer", "viewer password long")
	w = authRequest(h, "GET", "/api/v1/users", "", viewerCookie, "")
	if w.Code != 403 {
		t.Fatalf("viewer list users: %d", w.Code)
	}
	_ = viewerCSRF
}

func loginForTest(t *testing.T, h http.Handler, username, password string) (*http.Cookie, string) {
	t.Helper()
	w := authRequest(h, "POST", "/api/v1/auth/login", `{"username":"`+username+`","password":"`+password+`"}`, nil, "")
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return w.Result().Cookies()[0], body.Data.CSRF
}

func authRequest(h http.Handler, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if csrf != "" {
		r.Header.Set("X-CSRF-Token", csrf)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
