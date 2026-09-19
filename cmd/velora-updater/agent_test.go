package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matta813/velora-dns/internal/update"
)

func testAgent(t *testing.T) *Agent {
	t.Helper()
	return &Agent{config: AgentConfig{BinaryPath: filepath.Join(t.TempDir(), "velora-dns")}, manager: update.NewManager(update.DefaultConfig())}
}

func TestReadCurrentVersion(t *testing.T) {
	agent := testAgent(t)
	if got := agent.readCurrentVersion(); got != "unknown" {
		t.Fatalf("missing version = %q", got)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(agent.config.BinaryPath), "VERSION"), []byte("v1.2.3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := agent.readCurrentVersion(); got != "v1.2.3" {
		t.Fatalf("version = %q", got)
	}
}

func TestUpdateHandlerRejectsInvalidRequests(t *testing.T) {
	agent := testAgent(t)
	for _, body := range []string{"not json", `{"action":"delete"}`} {
		recorder := httptest.NewRecorder()
		agent.handleUpdate(recorder, httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"status":"error"`) {
			t.Fatalf("body %q: status=%d response=%s", body, recorder.Code, recorder.Body.String())
		}
	}
}

func TestStatusHandlerReportsIdle(t *testing.T) {
	agent := testAgent(t)
	recorder := httptest.NewRecorder()
	agent.handleStatus(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"state":"idle"`)) {
		t.Fatalf("status=%d response=%s", recorder.Code, recorder.Body.String())
	}
}

func TestEnvOrDefaultAndSplitLines(t *testing.T) {
	t.Setenv("VELORA_TEST_VALUE", "configured")
	if got := envOrDefault("VELORA_TEST_VALUE", "default"); got != "configured" {
		t.Fatalf("configured value = %q", got)
	}
	if got := envOrDefault("VELORA_ABSENT_VALUE", "default"); got != "default" {
		t.Fatalf("default value = %q", got)
	}
	if got := splitLines("one\ntwo\nthree"); len(got) != 3 || got[2] != "three" {
		t.Fatalf("splitLines = %#v", got)
	}
}
