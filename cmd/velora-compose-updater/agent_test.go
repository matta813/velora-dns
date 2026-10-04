package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matta813/velora-dns/internal/update"
)

func TestComposeArgsAndCurrentVersion(t *testing.T) {
	dir := t.TempDir()
	agent := &Agent{config: AgentConfig{ComposePath: "docker compose", ComposeProject: "velora", ComposeDir: dir}, manager: update.NewManager(update.DefaultConfig())}
	if got := agent.composeArgs("ps", "-q"); strings.Join(got, " ") != "docker compose -p velora ps -q" {
		t.Fatalf("compose args = %#v", got)
	}
	if got := agent.readCurrentVersion(); got != "unknown" {
		t.Fatalf("missing version = %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "RELEASE"), []byte("v2.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := agent.readCurrentVersion(); got != "v2.0.0" {
		t.Fatalf("version = %q", got)
	}
}

func TestComposeUpdateHandlerRejectsInvalidRequests(t *testing.T) {
	agent := &Agent{manager: update.NewManager(update.DefaultConfig())}
	for _, body := range []string{"invalid", `{"action":"rollback"}`} {
		recorder := httptest.NewRecorder()
		agent.handleUpdate(recorder, httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"status":"error"`) {
			t.Fatalf("body %q: status=%d response=%s", body, recorder.Code, recorder.Body.String())
		}
	}
}

func TestComposeEnvOrDefault(t *testing.T) {
	t.Setenv("VELORA_TEST_VALUE", "configured")
	if got := envOrDefault("VELORA_TEST_VALUE", "default"); got != "configured" {
		t.Fatalf("configured value = %q", got)
	}
	if got := envOrDefault("VELORA_ABSENT_VALUE", "default"); got != "default" {
		t.Fatalf("default value = %q", got)
	}
}

func TestComposeCheckAndConcurrentRequest(t *testing.T) {
	agent := &Agent{config: AgentConfig{Channel: "beta"}, manager: update.NewManager(update.DefaultConfig())}
	agent.resolve = func(context.Context, string) (update.Release, error) {
		return update.Release{Version: "2.0.0-beta.1", Channel: "beta", Architecture: "linux/arm64"}, nil
	}
	recorder := httptest.NewRecorder()
	agent.handleCheck(recorder, httptest.NewRequest(http.MethodGet, "/check", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"latest_version":"2.0.0-beta.1"`) {
		t.Fatalf("check: %d %s", recorder.Code, recorder.Body.String())
	}
	if _, err := agent.manager.Begin("1.0.0", "2.0.0-beta.1", "compose", "beta"); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	agent.handleUpdate(recorder, httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(`{"action":"update"}`)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("concurrent: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestComposeHealthHandlerReportsReady(t *testing.T) {
	agent := &Agent{manager: update.NewManager(update.DefaultConfig())}
	recorder := httptest.NewRecorder()
	agent.handleHealth(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"ready":true`) {
		t.Fatalf("status=%d response=%s", recorder.Code, recorder.Body.String())
	}
}
