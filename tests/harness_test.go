package tests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/app"
	"github.com/matta813/velora-dns/internal/config"
	wire "github.com/miekg/dns"
)

// server is one Velora process started in-process with real listeners. Its
// log goes to the test output so failures keep useful diagnostics.
type instance struct {
	t          *testing.T
	cfg        config.Config
	configPath string
	base       string
	client     *http.Client
	cancel     context.CancelFunc
	done       chan error
	cookie     string
	csrf       string
}

const adminPassword = "test admin password"

// newServer prepares a fresh installation; mutate adjusts its configuration.
func newServer(t *testing.T, mutate func(*config.Config)) *instance {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DNS.Listen = []string{unusedTCPAddress(t)}
	cfg.DNS.Upstreams = []string{"127.0.0.1:9"}
	cfg.HTTP.Listen = unusedTCPAddress(t)
	cfg.DatabasePath = filepath.Join(dir, "velora.db")
	cfg.QueryLog.Enabled = true
	cfg.Management.BootstrapUsername = "admin"
	cfg.Management.BootstrapPassword = adminPassword
	if mutate != nil {
		mutate(&cfg)
	}
	s := &instance{t: t, cfg: cfg, configPath: filepath.Join(dir, "config.yaml"), base: "http://" + cfg.HTTP.Listen, client: &http.Client{Timeout: 20 * time.Second}}
	if err := cfg.Save(s.configPath); err != nil {
		t.Fatal(err)
	}
	return s
}

// start runs the server until it is ready.
func (s *instance) start() {
	s.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan error, 1)
	cfg := s.cfg
	go func() {
		s.done <- app.Run(ctx, cfg, s.configPath, slog.New(slog.NewTextHandler(s.t.Output(), nil)), api.Version{Version: "e2e"})
	}()
	s.t.Cleanup(s.stop)
	deadline := time.Now().Add(15 * time.Second)
	for {
		response, err := s.client.Get(s.base + "/ready")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case err := <-s.done:
			s.done <- err
			s.t.Fatalf("server stopped before readiness: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("server did not become ready: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// stop shuts the server down (idempotent) and returns its exit error.
func (s *instance) stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	select {
	case err := <-s.done:
		if err != nil && !errors.Is(err, app.ErrRestartRequested) {
			s.t.Errorf("server shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		s.t.Error("server did not stop")
	}
	s.cancel = nil
}

// waitExit waits for the server to stop on its own (for example after a
// restart request) and returns its result.
func (s *instance) waitExit(timeout time.Duration) error {
	s.t.Helper()
	select {
	case err := <-s.done:
		s.cancel()
		s.cancel = nil
		return err
	case <-time.After(timeout):
		s.t.Fatal("server did not exit")
		return nil
	}
}

func (s *instance) login(username, password string) {
	s.t.Helper()
	s.cookie, s.csrf = "", ""
	status, headers, payload := s.request(http.MethodPost, "/api/v1/auth/login", `{"username":"`+username+`","password":"`+password+`"}`)
	if status != 200 {
		s.t.Fatalf("login %s: %d %s", username, status, payload)
	}
	for _, value := range headers.Values("Set-Cookie") {
		if strings.HasPrefix(value, "velora_session=") {
			s.cookie = strings.SplitN(value, ";", 2)[0]
		}
	}
	var body struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &body); err != nil || s.cookie == "" || body.Data.CSRFToken == "" {
		s.t.Fatalf("login response: %s", payload)
	}
	s.csrf = body.Data.CSRFToken
}

func (s *instance) request(method, path, body string) (int, http.Header, []byte) {
	s.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, s.base+path, reader)
	if err != nil {
		s.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return s.do(req)
}

func (s *instance) do(req *http.Request) (int, http.Header, []byte) {
	s.t.Helper()
	if s.cookie != "" {
		req.Header.Set("Cookie", s.cookie)
	}
	if s.csrf != "" && req.Method != http.MethodGet {
		req.Header.Set("X-CSRF-Token", s.csrf)
	}
	response, err := s.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return response.StatusCode, response.Header, payload
}

// must performs a request and decodes its data envelope into out.
func (s *instance) must(method, path, body string, want int, out any) {
	s.t.Helper()
	status, _, payload := s.request(method, path, body)
	if status != want {
		s.t.Fatalf("%s %s: HTTP %d, want %d: %s", method, path, status, want, payload)
	}
	if out != nil {
		envelope := struct {
			Data json.RawMessage `json:"data"`
		}{}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			s.t.Fatalf("%s %s: %v", method, path, err)
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			s.t.Fatalf("%s %s data: %v: %s", method, path, err, payload)
		}
	}
}

// query sends a real DNS query to the server over UDP.
func (s *instance) query(name string, qtype uint16) *wire.Msg {
	s.t.Helper()
	message := new(wire.Msg)
	message.SetQuestion(wire.Fqdn(name), qtype)
	client := &wire.Client{Timeout: 2 * time.Second}
	response, _, err := client.Exchange(message, s.cfg.DNS.Listen[0])
	if err != nil {
		s.t.Fatalf("DNS query %s: %v", name, err)
	}
	return response
}

func (s *instance) answersA(name string) []string {
	s.t.Helper()
	var out []string
	for _, rr := range s.query(name, wire.TypeA).Answer {
		if a, ok := rr.(*wire.A); ok {
			out = append(out, a.A.String())
		}
	}
	return out
}

// createZone adds a zone with one A record through the management API.
func (s *instance) createZone(zone, host, address string) {
	s.t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name": zone, "primary_ns": "ns." + zone, "contact": "hostmaster." + zone,
		"records": []map[string]any{{"name": host, "type": "A", "ttl": 60, "value": address}},
	})
	s.must(http.MethodPost, "/api/v1/zones", string(body), http.StatusCreated, nil)
}
