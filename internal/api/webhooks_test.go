package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/webhooks"
)

func TestWebhookEndpointsAreAdminOnlyAndHideTokens(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "hooks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, user := range [][2]string{{"admin", "admin"}, {"operator", "operator"}} {
		if _, err = db.CreateUser(ctx, user[0], user[0]+" password long", user[1]); err != nil {
			t.Fatal(err)
		}
	}
	hooks, err := webhooks.NewService(ctx, db, webhooks.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var authorization string
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer endpoint.Close()
	c := cache.New(10)
	h := New(Dependencies{Database: db, Auth: db, Events: db, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now(), Webhooks: hooks})
	operatorCookie, _ := loginForTest(t, h, "operator", "operator password long")
	if w := authRequest(h, "GET", "/api/v1/webhooks", "", operatorCookie, ""); w.Code != 403 {
		t.Fatalf("operator list: %d", w.Code)
	}
	adminCookie, adminCSRF := loginForTest(t, h, "admin", "admin password long")
	body := `{"name":"Home Assistant","url":"` + endpoint.URL + `/hook","allow_private":true,"events":["backup.failed"],"token":"abc123"}`
	w := authRequest(h, "POST", "/api/v1/webhooks", body, adminCookie, adminCSRF)
	if w.Code != 201 || strings.Contains(w.Body.String(), "abc123") || !strings.Contains(w.Body.String(), `"has_token":true`) {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w = authRequest(h, "POST", "/api/v1/webhooks", `{"name":"Private","url":"http://10.0.0.1/"}`, adminCookie, adminCSRF); w.Code != 400 {
		t.Fatalf("private without opt-in: %d", w.Code)
	}
	w = authRequest(h, "POST", "/api/v1/webhooks/1/test", "", adminCookie, adminCSRF)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) || authorization != "Bearer abc123" {
		t.Fatalf("test: %d %s auth=%q", w.Code, w.Body.String(), authorization)
	}
	// Updating without a token keeps it; an empty string removes it.
	w = authRequest(h, "PUT", "/api/v1/webhooks/1", `{"name":"Home Assistant","url":"`+endpoint.URL+`/hook","allow_private":true,"enabled":false}`, adminCookie, adminCSRF)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"has_token":true`) || !strings.Contains(w.Body.String(), `"last_status":"HTTP 204"`) {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	w = authRequest(h, "PUT", "/api/v1/webhooks/1", `{"name":"Home Assistant","url":"`+endpoint.URL+`/hook","allow_private":true,"token":""}`, adminCookie, adminCSRF)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"has_token":false`) {
		t.Fatalf("clear token: %d %s", w.Code, w.Body.String())
	}
	if w = authRequest(h, "GET", "/api/v1/webhooks/event-types", "", adminCookie, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "upstream.unavailable") {
		t.Fatalf("event types: %d %s", w.Code, w.Body.String())
	}
	if w = authRequest(h, "POST", "/api/v1/webhooks/9/test", "", adminCookie, adminCSRF); w.Code != 404 {
		t.Fatalf("missing test: %d", w.Code)
	}
	if w = authRequest(h, "DELETE", "/api/v1/webhooks/1", "", adminCookie, adminCSRF); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
}
