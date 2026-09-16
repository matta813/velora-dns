package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/metrics"
)

func TestManagementAuthenticationCSRFAndRoles(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "auth.db"))
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
