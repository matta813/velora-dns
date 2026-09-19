package api

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/deployment"
	"github.com/matta813/velora-dns/internal/metrics"
)

type fakeDeployment struct {
	settings deployment.Settings
	applied  bool
}

func (f *fakeDeployment) Get(context.Context) (deployment.Response, error) {
	return deployment.Response{Settings: f.settings}, nil
}
func (f *fakeDeployment) Apply(_ context.Context, s deployment.Settings) (deployment.Response, error) {
	f.settings = s
	f.applied = true
	return deployment.Response{Settings: s, Applied: true}, nil
}

func TestDeploymentSettingsRequireAdminAndValidateInput(t *testing.T) {
	db, err := database.Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, user := range []struct{ name, role string }{{"admin", "admin"}, {"operator", "operator"}} {
		if _, err := db.CreateUser(context.Background(), user.name, "long enough password", user.role); err != nil {
			t.Fatal(err)
		}
	}
	c := cache.New(10)
	agent := &fakeDeployment{settings: deployment.Settings{Channel: "stable", WebUIExposure: "local", DNSExposure: "local"}}
	h := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now(), Deployment: agent})
	operator, csrf := loginForTest(t, h, "operator", "long enough password")
	if w := authRequest(h, http.MethodGet, "/api/v1/deployment/settings", "", operator, csrf); w.Code != http.StatusForbidden {
		t.Fatalf("operator GET = %d", w.Code)
	}
	admin, adminCSRF := loginForTest(t, h, "admin", "long enough password")
	if w := authRequest(h, http.MethodPut, "/api/v1/deployment/settings", `{"channel":"nightly","web_ui_exposure":"local","dns_exposure":"local"}`, admin, adminCSRF); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT = %d", w.Code)
	}
	if w := authRequest(h, http.MethodPut, "/api/v1/deployment/settings", `{"channel":"beta","web_ui_exposure":"lan","dns_exposure":"lan"}`, admin, adminCSRF); w.Code != http.StatusOK || !agent.applied {
		t.Fatalf("valid PUT = %d %s", w.Code, w.Body.String())
	}
}
