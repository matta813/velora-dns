package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryStore struct {
	mu    sync.Mutex
	next  int64
	hooks map[int64]Hook
}

func (m *memoryStore) LoadWebhooks(context.Context) ([]Hook, error) { return nil, nil }
func (m *memoryStore) SaveWebhook(_ context.Context, hook Hook) (Hook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if hook.ID == 0 {
		m.next++
		hook.ID = m.next
	}
	m.hooks[hook.ID] = hook
	return hook, nil
}
func (m *memoryStore) DeleteWebhook(_ context.Context, id int64) error {
	delete(m.hooks, id)
	return nil
}

func newService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(context.Background(), &memoryStore{hooks: map[int64]Hook{}}, Options{Instance: Instance{Name: "test", Version: "1.2.3"}})
	if err != nil {
		t.Fatal(err)
	}
	service.delays = []time.Duration{time.Millisecond, time.Millisecond}
	return service
}

func TestDeliveryPayloadFilteringAndRetry(t *testing.T) {
	var mu sync.Mutex
	var received []Payload
	var headers []http.Header
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable) // retried
			return
		}
		var payload Payload
		_ = json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		received = append(received, payload)
		headers = append(headers, r.Header.Clone())
		mu.Unlock()
	}))
	defer server.Close()
	service := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { service.Run(ctx); close(done) }()
	hook, err := service.Create(ctx, Hook{Name: "Ops", URL: server.URL + "/hook", Events: []string{"upstream.unavailable"}, MinSeverity: "warning", AllowPrivate: true, Enabled: true, Token: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	if !hook.HasToken {
		t.Fatal("has_token should be reported")
	}
	if encoded, _ := json.Marshal(hook); strings.Contains(string(encoded), "s3cret") {
		t.Fatalf("token leaked: %s", encoded)
	}
	service.Publish(Event{Type: "upstream.recovered", Severity: "info", Title: "ignored type"})
	service.Publish(Event{Type: "upstream.unavailable", Severity: "info", Title: "ignored severity"})
	service.Publish(Event{Type: "upstream.unavailable", Severity: "warning", Title: "Upstream unavailable", Message: "192.0.2.1:53 is not responding", Link: "/"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if len(received) != 1 || calls.Load() != 2 {
		t.Fatalf("deliveries=%d calls=%d", len(received), calls.Load())
	}
	got := received[0]
	if got.Version != 1 || got.Type != "upstream.unavailable" || got.Severity != "warning" || got.Instance.Version != "1.2.3" || got.ID == "" || got.OccurredAt.IsZero() {
		t.Fatalf("payload: %+v", got)
	}
	if headers[0].Get("Authorization") != "Bearer s3cret" || headers[0].Get("X-Velora-Event") != "upstream.unavailable" || headers[0].Get("Content-Type") != "application/json" {
		t.Fatalf("headers: %v", headers[0])
	}
	status, _ := service.Get(hook.ID)
	if status.LastStatus != "HTTP 200" || status.ConsecutiveFailures != 0 || status.LastDeliveryAt == nil {
		t.Fatalf("status: %+v", status)
	}
}

func TestTestActionReportsErrorsWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Velora-Event") != "webhook.test" {
			t.Errorf("event header %q", r.Header.Get("X-Velora-Event"))
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	service := newService(t)
	hook, err := service.Create(context.Background(), Hook{Name: "Chat", URL: server.URL, AllowPrivate: true, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Test(context.Background(), hook.ID)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") || calls.Load() != 1 {
		t.Fatalf("test: %v calls=%d", err, calls.Load())
	}
	if updated.LastStatus != "failed" || updated.ConsecutiveFailures != 1 || !strings.Contains(updated.LastError, "403") {
		t.Fatalf("status: %+v", updated)
	}
	if _, err = service.Test(context.Background(), 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestDestinationChecks(t *testing.T) {
	for _, tc := range []struct {
		url          string
		allowPrivate bool
		ok           bool
	}{
		{"https://hooks.example.com/a", false, true},
		{"http://192.168.1.10:8123/api/webhook/x", false, false},
		{"http://192.168.1.10:8123/api/webhook/x", true, true},
		{"http://localhost:8080/", false, false},
		{"http://[::1]:8080/", true, true},
		{"http://169.254.169.254/latest/meta-data", true, false},
		{"http://0.0.0.0/", true, false},
		{"ftp://hooks.example.com/", false, false},
		{"https://user:pass@hooks.example.com/", false, false},
		{"https://hooks.example.com/#frag", false, false},
	} {
		if err := ValidateURL(tc.url, tc.allowPrivate); (err == nil) != tc.ok {
			t.Errorf("%s allowPrivate=%v: %v", tc.url, tc.allowPrivate, err)
		}
	}
	// Hostnames are checked again when delivering, after resolution.
	service := newService(t)
	service.options.Resolve = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.5")}}, nil
	}
	service.options.Dial = func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("must not dial a rejected address")
		return nil, nil
	}
	hook, err := service.Create(context.Background(), Hook{Name: "Rebind", URL: "https://hooks.example.com/", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Test(context.Background(), hook.ID); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("rebinding: %v", err)
	}
}

func TestValidationAndTokenHandling(t *testing.T) {
	ctx := context.Background()
	service := newService(t)
	for _, bad := range []Hook{
		{Name: "", URL: "https://a.example/"},
		{Name: "x", URL: "https://a.example/", Events: []string{"dns.everything"}},
		{Name: "x", URL: "https://a.example/", MinSeverity: "debug"},
		{Name: "x", URL: "https://a.example/", Token: "has space"},
	} {
		if _, err := service.Create(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%+v: %v", bad, err)
		}
	}
	hook, err := service.Create(ctx, Hook{Name: "Pager", URL: "https://a.example/", Token: "abc", Events: []string{"backup.failed", "backup.failed"}})
	if err != nil || hook.MinSeverity != "info" || len(hook.Events) != 1 {
		t.Fatalf("%+v %v", hook, err)
	}
	if _, err = service.Create(ctx, Hook{Name: "pager", URL: "https://b.example/"}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	updated, err := service.Update(ctx, hook.ID, Hook{Name: "Pager", URL: "https://c.example/"}, true)
	if err != nil || !updated.HasToken || updated.Token != "abc" {
		t.Fatalf("keep token: %+v %v", updated, err)
	}
	updated, err = service.Update(ctx, hook.ID, Hook{Name: "Pager", URL: "https://c.example/"}, false)
	if err != nil || updated.HasToken {
		t.Fatalf("clear token: %+v %v", updated, err)
	}
	if err = service.Delete(ctx, hook.ID); err != nil {
		t.Fatal(err)
	}
	if err = service.Delete(ctx, hook.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete: %v", err)
	}
}
