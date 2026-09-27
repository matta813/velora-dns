package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/app"
	"github.com/matta813/velora-dns/internal/backup"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/update"
	wire "github.com/miekg/dns"
)

// Create a backup, change the installation, restore through the API, let the
// server restart as its supervisor would, and verify DNS answers the backup.
func TestBackupRestoreWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end listener test")
	}
	s := newServer(t, nil)
	s.start()
	s.login("admin", adminPassword)
	s.createZone("before.test.", "www.before.test.", "192.0.2.10")

	const passphrase = "an e2e backup passphrase"
	status, _, bundle := s.request(http.MethodPost, "/api/v1/backup/create", `{"passphrase":"`+passphrase+`"}`)
	if status != 200 || len(bundle) < 100 {
		t.Fatalf("backup: %d (%d bytes)", status, len(bundle))
	}
	s.createZone("after.test.", "www.after.test.", "192.0.2.20")
	if got := s.answersA("www.after.test"); len(got) != 1 {
		t.Fatalf("after zone before restore: %v", got)
	}

	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	_ = form.WriteField("passphrase", passphrase)
	part, _ := form.CreateFormFile("bundle", "backup.vdns")
	_, _ = part.Write(bundle)
	_ = form.Close()
	req, _ := http.NewRequest(http.MethodPost, s.base+"/api/v1/backup/inspect", body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	status, _, payload := s.do(req)
	var inspection backup.Inspection
	if err := json.Unmarshal(payload, &struct {
		Data *backup.Inspection `json:"data"`
	}{&inspection}); err != nil || status != 200 || inspection.Summary.Zones != 1 {
		t.Fatalf("inspect: %d %s", status, payload)
	}
	// Inspecting changes nothing.
	if got := s.answersA("www.after.test"); len(got) != 1 {
		t.Fatal("inspection changed the running installation")
	}
	s.must(http.MethodPost, "/api/v1/backup/restore", `{"token":"`+inspection.Token+`","confirm":true}`, http.StatusAccepted, nil)
	if err := s.waitExit(10 * time.Second); !errors.Is(err, app.ErrRestartRequested) {
		t.Fatalf("expected a restart request, got %v", err)
	}

	// The supervisor starts Velora again, which applies the pending restore.
	changed, err := backup.PrepareStartup(context.Background(), s.configPath, s.cfg.DatabasePath)
	if err != nil || !changed {
		t.Fatalf("apply restore on start: %v %v", changed, err)
	}
	if s.cfg, err = config.Load(s.configPath); err != nil {
		t.Fatal(err)
	}
	s.start()
	if got := s.answersA("www.before.test"); len(got) != 1 || got[0] != "192.0.2.10" {
		t.Fatalf("restored zone: %v", got)
	}
	if got := s.answersA("www.after.test"); len(got) != 0 {
		t.Fatalf("zone created after the backup survived: %v", got)
	}
	s.login("admin", adminPassword)
	var state backup.RestoreState
	s.must(http.MethodGet, "/api/v1/backup/restore", "", 200, &state)
	if state.State != "completed" || state.SafetyCopy == "" {
		t.Fatalf("restore state: %+v", state)
	}
}

// fakeAgent is a deterministic updater agent on a Unix socket.
type fakeAgent struct {
	mu       sync.Mutex
	status   update.Status
	requests int
}

func startFakeAgent(t *testing.T) (*fakeAgent, string) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	agent := &fakeAgent{status: update.Status{State: update.StateIdle, Installed: "1.0.0"}}
	mux := http.NewServeMux()
	encode := func(w http.ResponseWriter, value any) { _ = json.NewEncoder(w).Encode(value) }
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { encode(w, update.Health{Ready: true}) })
	mux.HandleFunc("GET /check", func(w http.ResponseWriter, _ *http.Request) {
		encode(w, update.CheckResult{Installed: "1.0.0", Latest: "1.1.0", UpdateAvailable: true, Channel: "stable", Architecture: "linux/amd64"})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		encode(w, agent.status)
	})
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, _ *http.Request) { encode(w, []update.Entry{}) })
	mux.HandleFunc("POST /update", func(w http.ResponseWriter, _ *http.Request) {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		agent.requests++
		agent.status = update.Status{State: update.StateInstalling, Installed: "1.0.0", FromVersion: "1.0.0", ToVersion: "1.1.0", Updating: true}
		w.WriteHeader(http.StatusAccepted)
		encode(w, update.RequestResponse{Status: "accepted", Version: "1.1.0"})
	})
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return agent, socket
}

// Exercise check → update → progress → completion against a local agent,
// including the guard against a second concurrent update.
func TestUpdaterWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end listener test")
	}
	agent, socket := startFakeAgent(t)
	t.Setenv("VELORA_UPDATER_SOCKET", socket)
	s := newServer(t, nil)
	s.start()
	s.login("admin", adminPassword)

	var check struct {
		UpdateAvailable bool   `json:"update_available"`
		Latest          string `json:"latest_version"`
	}
	s.must(http.MethodGet, "/api/v1/update/check", "", 200, &check)
	if !check.UpdateAvailable || check.Latest != "1.1.0" {
		t.Fatalf("check: %+v", check)
	}
	s.must(http.MethodPost, "/api/v1/update/request", `{"action":"update"}`, http.StatusAccepted, nil)
	var status update.Status
	s.must(http.MethodGet, "/api/v1/update/status", "", 200, &status)
	if !status.Updating || status.State != update.StateInstalling || status.ToVersion != "1.1.0" {
		t.Fatalf("status while updating: %+v", status)
	}
	if code, _, payload := s.request(http.MethodPost, "/api/v1/update/request", `{"action":"update"}`); code != http.StatusConflict || !strings.Contains(string(payload), "update_in_progress") {
		t.Fatalf("second update: %d %s", code, payload)
	}
	agent.mu.Lock()
	agent.status = update.Status{State: update.StateCompleted, Installed: "1.1.0", Updating: false, ReadinessOK: true}
	requests := agent.requests
	agent.mu.Unlock()
	s.must(http.MethodGet, "/api/v1/update/status", "", 200, &status)
	if status.Updating || status.Installed != "1.1.0" || requests != 1 {
		t.Fatalf("completed: %+v requests=%d", status, requests)
	}
	// Viewers can read update state but not start updates.
	s.must(http.MethodPost, "/api/v1/users", `{"username":"viewer","password":"viewer password long","role":"viewer"}`, http.StatusCreated, nil)
	s.login("viewer", "viewer password long")
	if code, _, _ := s.request(http.MethodPost, "/api/v1/update/request", `{"action":"update"}`); code != http.StatusForbidden {
		t.Fatalf("viewer update: %d", code)
	}
}

// A client exceeding its query budget gets SERVFAIL and the rejection is counted.
func TestRateLimitWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end listener test")
	}
	s := newServer(t, nil)
	s.start()
	s.login("admin", adminPassword)
	s.createZone("limits.test.", "www.limits.test.", "192.0.2.30")
	if got := s.answersA("www.limits.test"); len(got) != 1 {
		t.Fatalf("baseline answer: %v", got)
	}
	s.must(http.MethodPut, "/api/v1/settings/rate-limit", `{"enabled":true,"global_qps":1000,"client_qps":1,"rate_limit_burst":2}`, 200, nil)
	limited := 0
	for i := 0; i < 20; i++ {
		if s.query("www.limits.test", wire.TypeA).Rcode == wire.RcodeServerFailure {
			limited++
		}
	}
	if limited < 10 {
		t.Fatalf("only %d of 20 burst queries were limited", limited)
	}
	var rateStatus struct {
		RejectedTotal uint64 `json:"rejected_total"`
	}
	s.must(http.MethodGet, "/api/v1/settings/rate-limit/status", "", 200, &rateStatus)
	if rateStatus.RejectedTotal < uint64(limited) {
		t.Fatalf("rejections: %d < %d", rateStatus.RejectedTotal, limited)
	}
	s.must(http.MethodPut, "/api/v1/settings/rate-limit", `{"enabled":false,"global_qps":1000,"client_qps":1,"rate_limit_burst":2}`, 200, nil)
	if got := s.answersA("www.limits.test"); len(got) != 1 {
		t.Fatalf("answer after disabling the limit: %v", got)
	}
}

// Two real processes: zones created on the primary are answered by the replica.
func TestClusterReplicationWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end listener test")
	}
	primary, replica := newServer(t, nil), newServer(t, nil)
	primary.start()
	replica.start()
	primary.login("admin", adminPassword)
	replica.login("admin", adminPassword)
	primary.createZone("home.test.", "nas.home.test.", "192.0.2.40")

	primary.must(http.MethodPost, "/api/v1/cluster/create", `{"name":"dns1","advertised_url":"`+primary.base+`","allow_insecure":true}`, http.StatusCreated, nil)
	var token struct {
		Token string `json:"token"`
	}
	primary.must(http.MethodPost, "/api/v1/cluster/join-tokens", "", http.StatusCreated, &token)
	replica.must(http.MethodPost, "/api/v1/cluster/connect", `{"primary_url":"`+primary.base+`","token":"`+token.Token+`","name":"dns2","allow_insecure":true}`, 200, nil)
	replica.must(http.MethodPost, "/api/v1/cluster/sync", "", 200, nil)
	if got := replica.answersA("nas.home.test"); len(got) != 1 || got[0] != "192.0.2.40" {
		t.Fatalf("replica answer: %v", got)
	}
	if code, _, payload := replica.request(http.MethodPost, "/api/v1/zones", `{"name":"local.test.","primary_ns":"ns.local.test.","contact":"h.local.test.","records":[]}`); code != http.StatusConflict || !strings.Contains(string(payload), "zones_managed_by_primary") {
		t.Fatalf("replica zone write: %d %s", code, payload)
	}

	primary.createZone("lab.test.", "ci.lab.test.", "192.0.2.41")
	replica.must(http.MethodPost, "/api/v1/cluster/sync", "", 200, nil)
	if got := replica.answersA("ci.lab.test"); len(got) != 1 || got[0] != "192.0.2.41" {
		t.Fatalf("replica did not receive the new zone: %v", got)
	}
	var overview struct {
		Members []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"members"`
	}
	replica.must(http.MethodPost, "/api/v1/cluster/sync", "", 200, nil)
	primary.must(http.MethodGet, "/api/v1/cluster/overview", "", 200, &overview)
	if len(overview.Members) != 1 || overview.Members[0].Name != "dns2" || overview.Members[0].Status != "in_sync" {
		t.Fatalf("primary view: %+v", overview)
	}
}
