package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matta813/velora-dns/internal/update"
)

func TestUpdateAPIForwardsToAgentSocket(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	agentMux := http.NewServeMux()
	agentMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(update.Health{Ready: true})
	})
	agentMux.HandleFunc("GET /check", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(update.CheckResult{Installed: "1.0.0", Latest: "1.1.0", UpdateAvailable: true, Channel: "stable"})
	})
	agentMux.HandleFunc("POST /update", func(w http.ResponseWriter, r *http.Request) {
		var request map[string]string
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request["action"] != "update" {
			t.Errorf("agent request = %#v", request)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(update.RequestResponse{Status: "accepted", Version: "1.1.0"})
	})
	agentServer := &http.Server{Handler: agentMux}
	defer agentServer.Close()
	go func() { _ = agentServer.Serve(listener) }()

	mux := http.NewServeMux()
	registerUpdate(mux, update.Client{SocketPath: socket}, Version{})
	for _, tc := range []struct {
		method, path, body, contains string
		status                       int
	}{
		{http.MethodGet, "/api/v1/update/health", "", `"ready":true`, http.StatusOK},
		{http.MethodGet, "/api/v1/update/check", "", `"latest_version":"1.1.0"`, http.StatusOK},
		{http.MethodPost, "/api/v1/update/request", `{"action":"update"}`, `"version":"1.1.0"`, http.StatusAccepted},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		mux.ServeHTTP(recorder, request)
		if recorder.Code != tc.status || !strings.Contains(recorder.Body.String(), tc.contains) {
			t.Fatalf("%s %s: status=%d body=%s", tc.method, tc.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestUpdaterFailureExplainsSocketProblems(t *testing.T) {
	for _, tc := range []struct {
		err     error
		message string
	}{
		{os.ErrPermission, "socket access was denied"},
		{os.ErrNotExist, "not running"},
	} {
		recorder := httptest.NewRecorder()
		updaterFailure(recorder, tc.err)
		if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), tc.message) {
			t.Fatalf("error %v: status=%d response=%s", tc.err, recorder.Code, recorder.Body.String())
		}
	}
}
