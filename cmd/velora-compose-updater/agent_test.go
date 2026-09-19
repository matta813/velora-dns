package main

import (
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
