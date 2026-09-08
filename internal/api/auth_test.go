package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/auth"
	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/metrics"
)

func authAPI(t *testing.T) (http.Handler, *database.Store) {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "auth-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	service := auth.New(db, time.Hour)
	if err = service.Bootstrap(context.Background(), "admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	c := cache.New(10)
	return New(Dependencies{Database: db, Auth: service, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now()}), db
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
func loginAPI(t *testing.T, h http.Handler, username, password string) (*http.Cookie, string) {
	t.Helper()
	w := authRequest(h, "POST", "/api/v1/auth/login", `{"username":"`+username+`","password":"`+password+`"}`, nil, "")
	if w.Code != 200 {
		t.Fatalf("login %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == sessionCookie {
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
				t.Fatal("insecure session cookie")
			}
			return cookie, payload.Data.CSRF
		}
	}
	t.Fatal("missing session cookie")
	return nil, ""
}
func TestAuthenticationCSRFAndRoles(t *testing.T) {
	h, _ := authAPI(t)
	if w := authRequest(h, "GET", "/api/v1/config", "", nil, ""); w.Code != 401 {
		t.Fatalf("unauthenticated: %d", w.Code)
	}
	if w := authRequest(h, "POST", "/api/v1/auth/login", `{"username":"admin","password":"wrong password here"}`, nil, ""); w.Code != 401 || strings.Contains(w.Body.String(), "wrong password here") {
		t.Fatalf("bad login: %d %s", w.Code, w.Body.String())
	}
	adminCookie, adminCSRF := loginAPI(t, h, "admin", "correct horse battery staple")
	if w := authRequest(h, "DELETE", "/api/v1/cache", "", adminCookie, ""); w.Code != 403 {
		t.Fatalf("CSRF bypass: %d", w.Code)
	}
	created := authRequest(h, "POST", "/api/v1/users", `{"username":"viewer","password":"another secure password","role":"viewer"}`, adminCookie, adminCSRF)
	if created.Code != 201 || strings.Contains(created.Body.String(), "argon2") || strings.Contains(created.Body.String(), "secure password") {
		t.Fatalf("create user: %d %s", created.Code, created.Body.String())
	}
	viewerCookie, viewerCSRF := loginAPI(t, h, "viewer", "another secure password")
	if w := authRequest(h, "GET", "/api/v1/config", "", viewerCookie, ""); w.Code != 200 {
		t.Fatalf("viewer read: %d", w.Code)
	}
	if w := authRequest(h, "DELETE", "/api/v1/cache", "", viewerCookie, viewerCSRF); w.Code != 403 {
		t.Fatalf("viewer mutation: %d", w.Code)
	}
	if w := authRequest(h, "GET", "/api/v1/users", "", viewerCookie, ""); w.Code != 403 {
		t.Fatalf("viewer user list: %d", w.Code)
	}
	if w := authRequest(h, "GET", "/api/v1/audit", "", adminCookie, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "session.login") {
		t.Fatalf("audit: %d %s", w.Code, w.Body.String())
	}
}

func TestLoginGuardBoundsAttemptsAndSourceMemory(t *testing.T) {
	guard := newLoginGuard()
	now := time.Unix(1000, 0)
	for i := range 2000 {
		release, ok := guard.acquire(fmt.Sprintf("192.0.2.%d", i), now)
		if !ok {
			t.Fatal("unexpected login guard rejection")
		}
		release()
	}
	if len(guard.entries) != 1024 || len(guard.order) != 1024 {
		t.Fatalf("unbounded guard: %d %d", len(guard.entries), len(guard.order))
	}
	for range 5 {
		release, ok := guard.acquire("198.51.100.1", now)
		if !ok {
			t.Fatal("early rejection")
		}
		release()
	}
	if release, ok := guard.acquire("198.51.100.1", now); ok {
		release()
		t.Fatal("sixth attempt accepted")
	}
	guard.success("198.51.100.1")
	if release, ok := guard.acquire("198.51.100.1", now); !ok {
		t.Fatal("successful login did not reset attempts")
	} else {
		release()
	}
}
