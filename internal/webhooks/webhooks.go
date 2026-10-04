// Package webhooks delivers operational events to external HTTP endpoints.
// Delivery is asynchronous and bounded: a slow or failing endpoint never
// blocks DNS processing or other endpoints.
package webhooks

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	PayloadVersion = 1
	MaxHooks       = 16
	maxNameLength  = 80
	maxURLLength   = 2048
	maxTokenLength = 512
	queueSize      = 256
	parallel       = 4
)

// Retry delays after a failed delivery; the attempt count is len+1.
var retryDelays = []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}

var (
	ErrInvalid  = errors.New("invalid webhook")
	ErrNotFound = errors.New("webhook not found")
	ErrExists   = errors.New("webhook already exists")
)

// EventTypes lists every event a webhook can subscribe to.
var EventTypes = []string{
	"upstream.unavailable",
	"upstream.recovered",
	"blocklist.refresh_failed",
	"blocklist.refresh_recovered",
	"backup.created",
	"backup.failed",
	"config.rollback",
	"cluster.sync_failed",
	"cluster.sync_recovered",
}

var severityRank = map[string]int{"info": 0, "warning": 1, "critical": 2}

// Hook is a configured endpoint. Token is write-only: it is sent as a bearer
// token and never returned by the API or written to logs.
type Hook struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	URL                 string     `json:"url"`
	Events              []string   `json:"events"`
	MinSeverity         string     `json:"min_severity"`
	AllowPrivate        bool       `json:"allow_private"`
	Enabled             bool       `json:"enabled"`
	Token               string     `json:"-"`
	HasToken            bool       `json:"has_token"`
	LastDeliveryAt      *time.Time `json:"last_delivery_at"`
	LastStatus          string     `json:"last_status"`
	LastError           string     `json:"last_error"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

// Event is what callers publish; Payload is what endpoints receive.
type Event struct {
	Type     string
	Severity string
	Title    string
	Message  string
	Link     string
}

type Payload struct {
	Version    int       `json:"version"`
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Severity   string    `json:"severity"`
	Title      string    `json:"title"`
	Message    string    `json:"message"`
	Link       string    `json:"link,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	Instance   Instance  `json:"instance"`
}

type Instance struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Store interface {
	LoadWebhooks(context.Context) ([]Hook, error)
	SaveWebhook(context.Context, Hook) (Hook, error)
	DeleteWebhook(context.Context, int64) error
}

// Dialer connects to a validated address; tests replace it.
type Dialer func(ctx context.Context, network, address string) (net.Conn, error)

type Resolver func(ctx context.Context, host string) ([]net.IPAddr, error)

type Options struct {
	Instance Instance
	Logger   *slog.Logger
	// Resolve and Dial default to the system resolver and a 3 s dialer.
	Resolve Resolver
	Dial    Dialer
}

type delivery struct {
	hook    Hook
	payload Payload
}

type Service struct {
	store    Store
	options  Options
	mu       sync.Mutex
	hooks    []Hook
	queue    chan delivery
	now      func() time.Time
	delays   []time.Duration
	inflight sync.WaitGroup
}

func NewService(ctx context.Context, store Store, options Options) (*Service, error) {
	hooks, err := store.LoadWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
	if options.Resolve == nil {
		options.Resolve = net.DefaultResolver.LookupIPAddr
	}
	if options.Dial == nil {
		options.Dial = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	}
	for i := range hooks {
		hooks[i].HasToken = hooks[i].Token != ""
	}
	return &Service{store: store, options: options, hooks: hooks, queue: make(chan delivery, queueSize), now: time.Now, delays: retryDelays}, nil
}

// Run delivers queued events until ctx ends, then waits briefly for
// in-flight deliveries.
func (s *Service) Run(ctx context.Context) {
	slots := make(chan struct{}, parallel)
	for {
		select {
		case <-ctx.Done():
			done := make(chan struct{})
			go func() { s.inflight.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
			}
			return
		case item := <-s.queue:
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				continue
			}
			s.inflight.Go(func() {
				defer func() { <-slots }()
				s.deliverWithRetry(ctx, item)
			})
		}
	}
}

// Publish queues the event for every matching enabled hook. It never blocks;
// when the queue is full the event is dropped and logged.
func (s *Service) Publish(event Event) {
	s.mu.Lock()
	hooks := slices.Clone(s.hooks)
	s.mu.Unlock()
	payload := s.payload(event)
	for _, hook := range hooks {
		if !hook.Enabled || !wants(hook, event) {
			continue
		}
		select {
		case s.queue <- delivery{hook: hook, payload: payload}:
		default:
			s.options.Logger.Warn("webhook queue full; event dropped", "webhook", hook.Name, "event", event.Type)
		}
	}
}

func wants(hook Hook, event Event) bool {
	if severityRank[event.Severity] < severityRank[hook.MinSeverity] {
		return false
	}
	return len(hook.Events) == 0 || slices.Contains(hook.Events, event.Type)
}

func (s *Service) payload(event Event) Payload {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	return Payload{
		Version: PayloadVersion, ID: hex.EncodeToString(id), Type: event.Type, Severity: event.Severity,
		Title: event.Title, Message: event.Message, Link: event.Link, OccurredAt: s.now().UTC(), Instance: s.options.Instance,
	}
}

func (s *Service) deliverWithRetry(ctx context.Context, item delivery) {
	for attempt := 0; ; attempt++ {
		status, err := s.send(ctx, item.hook, item.payload)
		s.record(item.hook.ID, status, err)
		if err == nil || attempt >= len(s.delays) || !retryable(status) {
			if err != nil {
				s.options.Logger.Warn("webhook delivery failed", "webhook", item.hook.Name, "event", item.payload.Type, "error", err)
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.delays[attempt]):
		}
	}
}

// retryable reports whether a failure may succeed later: network errors,
// rate limiting and server errors are retried, other client errors are not.
func retryable(status int) bool {
	return status == 0 || status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500
}

// send performs one delivery. The transport resolves and validates the
// destination inside DialContext and connects to that exact address, so DNS
// rebinding or an environment proxy cannot bypass the address checks.
func (s *Service) send(parent context.Context, hook Hook, payload Payload) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxResponseHeaderBytes: 16 << 10, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 4 * time.Second}
	defer transport.CloseIdleConnections()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := s.options.Resolve(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("destination could not be resolved")
		}
		if len(ips) > 16 {
			ips = ips[:16]
		}
		for _, ip := range ips {
			addr, ok := netip.AddrFromSlice(ip.IP)
			if !ok {
				return nil, fmt.Errorf("destination address is invalid")
			}
			if err := checkAddress(addr.Unmap(), hook.AllowPrivate); err != nil {
				return nil, err
			}
		}
		var last error
		for _, ip := range ips {
			conn, err := s.options.Dial(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, fmt.Errorf("connection failed: %w", last)
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Velora-DNS-Webhook/"+strings.TrimSpace(s.options.Instance.Version))
	req.Header.Set("X-Velora-Event", payload.Type)
	req.Header.Set("X-Velora-Delivery", payload.ID)
	if hook.Token != "" {
		req.Header.Set("Authorization", "Bearer "+hook.Token)
	}
	response, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, fmt.Errorf("timed out")
		}
		return 0, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	_ = response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return response.StatusCode, fmt.Errorf("endpoint returned HTTP %d", response.StatusCode)
	}
	return response.StatusCode, nil
}

func (s *Service) record(id int64, status int, err error) {
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.hooks {
		if s.hooks[i].ID != id {
			continue
		}
		hook := &s.hooks[i]
		hook.LastDeliveryAt = &now
		if err == nil {
			hook.LastStatus, hook.LastError, hook.ConsecutiveFailures = fmt.Sprintf("HTTP %d", status), "", 0
		} else {
			hook.LastStatus, hook.LastError = "failed", truncate(err.Error(), 200)
			hook.ConsecutiveFailures++
		}
	}
}

// Test sends one webhook.test event immediately, without retries.
func (s *Service) Test(ctx context.Context, id int64) (Hook, error) {
	s.mu.Lock()
	i := slices.IndexFunc(s.hooks, func(hook Hook) bool { return hook.ID == id })
	var hook Hook
	if i >= 0 {
		hook = s.hooks[i]
	}
	s.mu.Unlock()
	if i < 0 {
		return Hook{}, ErrNotFound
	}
	payload := s.payload(Event{Type: "webhook.test", Severity: "info", Title: "Test notification", Message: "Velora DNS can reach this webhook."})
	status, err := s.send(ctx, hook, payload)
	s.record(id, status, err)
	updated, _ := s.Get(id)
	return updated, err
}

func (s *Service) Get(id int64) (Hook, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, hook := range s.hooks {
		if hook.ID == id {
			return hook, true
		}
	}
	return Hook{}, false
}

func (s *Service) List(context.Context) ([]Hook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.hooks), nil
}

func (s *Service) Create(ctx context.Context, hook Hook) (Hook, error) {
	hook.ID = 0
	return s.save(ctx, hook, false)
}

// Update replaces a hook. keepToken keeps the stored token when the caller
// did not send a new one.
func (s *Service) Update(ctx context.Context, id int64, hook Hook, keepToken bool) (Hook, error) {
	if id < 1 {
		return Hook{}, ErrNotFound
	}
	hook.ID = id
	return s.save(ctx, hook, keepToken)
}

func (s *Service) save(ctx context.Context, hook Hook, keepToken bool) (Hook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	if hook.ID != 0 {
		if index = slices.IndexFunc(s.hooks, func(existing Hook) bool { return existing.ID == hook.ID }); index < 0 {
			return Hook{}, ErrNotFound
		}
		if keepToken {
			hook.Token = s.hooks[index].Token
		}
	} else if len(s.hooks) >= MaxHooks {
		return Hook{}, fmt.Errorf("%w: at most %d webhooks", ErrInvalid, MaxHooks)
	}
	normalized, err := normalize(hook)
	if err != nil {
		return Hook{}, err
	}
	for i, existing := range s.hooks {
		if i != index && strings.EqualFold(existing.Name, normalized.Name) {
			return Hook{}, fmt.Errorf("%w: a webhook named %q already exists", ErrExists, existing.Name)
		}
	}
	saved, err := s.store.SaveWebhook(ctx, normalized)
	if err != nil {
		return Hook{}, err
	}
	saved.HasToken = saved.Token != ""
	if index < 0 {
		s.hooks = append(slices.Clone(s.hooks), saved)
	} else {
		// Delivery status survives edits.
		saved.LastDeliveryAt, saved.LastStatus, saved.LastError, saved.ConsecutiveFailures = s.hooks[index].LastDeliveryAt, s.hooks[index].LastStatus, s.hooks[index].LastError, s.hooks[index].ConsecutiveFailures
		s.hooks = slices.Clone(s.hooks)
		s.hooks[index] = saved
	}
	return saved, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.hooks, func(hook Hook) bool { return hook.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	if err := s.store.DeleteWebhook(ctx, id); err != nil {
		return err
	}
	s.hooks = slices.Delete(slices.Clone(s.hooks), i, i+1)
	return nil
}

func normalize(hook Hook) (Hook, error) {
	hook.Name = strings.TrimSpace(hook.Name)
	if hook.Name == "" || len(hook.Name) > maxNameLength || strings.ContainsAny(hook.Name, "\r\n\x00") {
		return Hook{}, fmt.Errorf("%w: name must be a single line of at most %d characters", ErrInvalid, maxNameLength)
	}
	hook.URL = strings.TrimSpace(hook.URL)
	if err := ValidateURL(hook.URL, hook.AllowPrivate); err != nil {
		return Hook{}, err
	}
	if len(hook.Token) > maxTokenLength || strings.ContainsAny(hook.Token, "\r\n\x00 ") {
		return Hook{}, fmt.Errorf("%w: token must be at most %d characters without spaces", ErrInvalid, maxTokenLength)
	}
	if hook.MinSeverity == "" {
		hook.MinSeverity = "info"
	}
	if _, ok := severityRank[hook.MinSeverity]; !ok {
		return Hook{}, fmt.Errorf("%w: minimum severity must be info, warning or critical", ErrInvalid)
	}
	events := []string{}
	for _, event := range hook.Events {
		if !slices.Contains(EventTypes, event) {
			return Hook{}, fmt.Errorf("%w: unknown event type %q", ErrInvalid, event)
		}
		if !slices.Contains(events, event) {
			events = append(events, event)
		}
	}
	hook.Events = events
	return hook, nil
}

// ValidateURL accepts http(s) URLs without embedded credentials. Literal
// addresses are checked here; hostnames are checked again at delivery time.
func ValidateURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > maxURLLength || u.User != nil || u.Hostname() == "" || u.Opaque != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%w: use an http:// or https:// URL without credentials", ErrInvalid)
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil {
		return checkAddress(ip.Unmap(), allowPrivate)
	}
	if strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), "localhost") && !allowPrivate {
		return fmt.Errorf("%w: private and loopback destinations require allow_private", ErrInvalid)
	}
	return nil
}

// checkAddress always rejects unspecified, multicast and link-local
// destinations (which include cloud metadata services). Private and loopback
// addresses are allowed only when the administrator opted in for this hook.
func checkAddress(addr netip.Addr, allowPrivate bool) error {
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() {
		return fmt.Errorf("%w: destination address %s is not allowed", ErrInvalid, addr)
	}
	if addr.IsPrivate() || addr.IsLoopback() || netip.MustParsePrefix("100.64.0.0/10").Contains(addr) {
		if !allowPrivate {
			return fmt.Errorf("%w: %s is a private or loopback address; enable allow_private to use it", ErrInvalid, addr)
		}
	}
	return nil
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
