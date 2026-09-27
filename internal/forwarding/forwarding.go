// Package forwarding sends queries for selected domains to dedicated
// upstream resolvers instead of the global upstream list.
package forwarding

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/filtering"
	wire "github.com/miekg/dns"
)

const (
	MaxRules            = 256
	MaxUpstreams        = 4
	maxDescription      = 200
	defaultUpstreamPort = 53
)

var (
	ErrInvalid  = errors.New("invalid forwarding rule")
	ErrNotFound = errors.New("forwarding rule not found")
	ErrExists   = errors.New("a forwarding rule for this domain already exists")
)

// Rule forwards the domain and every name below it to Upstreams.
type Rule struct {
	ID          int64    `json:"id"`
	Domain      string   `json:"domain"`
	Upstreams   []string `json:"upstreams"`
	Enabled     bool     `json:"enabled"`
	Description string   `json:"description"`
}

// RuleStatus is a rule with the live health of its upstreams.
type RuleStatus struct {
	Rule
	Health []dns.UpstreamStatus `json:"health"`
}

type Store interface {
	LoadForwardRules(context.Context) ([]Rule, error)
	SaveForwardRule(context.Context, Rule) (Rule, error)
	DeleteForwardRule(context.Context, int64) error
}

type Options struct {
	Timeout time.Duration
	Retries int
	// Listen holds the resolver's own DNS listen addresses; rules that would
	// forward back to Velora itself are rejected to prevent query loops.
	Listen []string
	// OnChange is called with a rule's domain after it is created, changed
	// or removed so stale cached answers can be dropped.
	OnChange func(domain string)
}

type route struct {
	rule      Rule
	forwarder *dns.Forwarder
	health    *dns.UpstreamHealth
}

type table map[string]*route

type Service struct {
	store   Store
	options Options
	self    []netip.AddrPort
	mu      sync.Mutex
	rules   []Rule
	current atomic.Pointer[table]
}

func NewService(ctx context.Context, store Store, options Options) (*Service, error) {
	if options.Timeout <= 0 {
		options.Timeout = 2 * time.Second
	}
	s := &Service{store: store, options: options}
	for _, raw := range options.Listen {
		if address, err := netip.ParseAddrPort(raw); err == nil {
			s.self = append(s.self, address)
		}
	}
	rules, err := store.LoadForwardRules(ctx)
	if err != nil {
		return nil, err
	}
	s.rules = rules
	s.current.Store(s.compile(rules, nil))
	return s, nil
}

// compile builds the lookup table. Health trackers of unchanged rules are
// carried over so a configuration edit elsewhere does not reset state.
func (s *Service) compile(rules []Rule, previous *table) *table {
	out := make(table, len(rules))
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		var health *dns.UpstreamHealth
		if previous != nil {
			if old, ok := (*previous)[rule.Domain]; ok && slices.Equal(old.rule.Upstreams, rule.Upstreams) {
				health = old.health
			}
		}
		if health == nil {
			health = dns.NewUpstreamHealth(rule.Upstreams)
		}
		out[rule.Domain] = &route{rule: rule, health: health, forwarder: s.forwarder(rule.Upstreams, health)}
	}
	return &out
}

// DNSSEC validation is intentionally not applied: conditionally forwarded
// names are typically private zones that cannot chain to the root.
func (s *Service) forwarder(upstreams []string, health *dns.UpstreamHealth) *dns.Forwarder {
	return &dns.Forwarder{Upstreams: upstreams, Timeout: s.options.Timeout, Retries: s.options.Retries, Health: health}
}

// match returns the most specific enabled rule covering name.
func (s *Service) match(name string) *route {
	routes := *s.current.Load()
	if len(routes) == 0 {
		return nil
	}
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	for name != "" {
		if r, ok := routes[name]; ok {
			return r
		}
		_, parent, found := strings.Cut(name, ".")
		if !found {
			break
		}
		name = parent
	}
	return nil
}

// Match reports which rule domain, if any, handles name.
func (s *Service) Match(name string) (string, bool) {
	if r := s.match(name); r != nil {
		return r.rule.Domain, true
	}
	return "", false
}

func (s *Service) List(context.Context) ([]RuleStatus, error) {
	s.mu.Lock()
	rules := slices.Clone(s.rules)
	s.mu.Unlock()
	routes := *s.current.Load()
	out := make([]RuleStatus, 0, len(rules))
	for _, rule := range rules {
		status := RuleStatus{Rule: rule, Health: []dns.UpstreamStatus{}}
		if r, ok := routes[rule.Domain]; ok {
			status.Health = r.health.Snapshot()
		}
		out = append(out, status)
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, rule Rule) (Rule, error) {
	rule.ID = 0
	return s.save(ctx, rule)
}

func (s *Service) Update(ctx context.Context, id int64, rule Rule) (Rule, error) {
	if id < 1 {
		return Rule{}, ErrNotFound
	}
	rule.ID = id
	return s.save(ctx, rule)
}

func (s *Service) save(ctx context.Context, rule Rule) (Rule, error) {
	normalized, err := s.normalize(rule)
	if err != nil {
		return Rule{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i, existing := range s.rules {
		if existing.ID == normalized.ID && normalized.ID != 0 {
			index = i
		} else if existing.Domain == normalized.Domain {
			return Rule{}, ErrExists
		}
	}
	if normalized.ID != 0 && index < 0 {
		return Rule{}, ErrNotFound
	}
	if normalized.ID == 0 && len(s.rules) >= MaxRules {
		return Rule{}, fmt.Errorf("%w: at most %d rules", ErrInvalid, MaxRules)
	}
	saved, err := s.store.SaveForwardRule(ctx, normalized)
	if err != nil {
		return Rule{}, err
	}
	candidate := slices.Clone(s.rules)
	var oldDomain string
	if index < 0 {
		candidate = append(candidate, saved)
	} else {
		oldDomain = candidate[index].Domain
		candidate[index] = saved
	}
	s.rules = candidate
	s.current.Store(s.compile(candidate, s.current.Load()))
	s.changed(saved.Domain)
	if oldDomain != "" && oldDomain != saved.Domain {
		s.changed(oldDomain)
	}
	return saved, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := slices.IndexFunc(s.rules, func(rule Rule) bool { return rule.ID == id })
	if index < 0 {
		return ErrNotFound
	}
	if err := s.store.DeleteForwardRule(ctx, id); err != nil {
		return err
	}
	domain := s.rules[index].Domain
	s.rules = slices.Delete(slices.Clone(s.rules), index, index+1)
	s.current.Store(s.compile(s.rules, s.current.Load()))
	s.changed(domain)
	return nil
}

func (s *Service) changed(domain string) {
	if s.options.OnChange != nil {
		s.options.OnChange(domain)
	}
}

func (s *Service) normalize(rule Rule) (Rule, error) {
	raw := strings.TrimSpace(rule.Domain)
	if strings.HasPrefix(raw, "*") {
		return Rule{}, fmt.Errorf("%w: rules already cover subdomains; enter the domain without a wildcard", ErrInvalid)
	}
	domain, err := filtering.NormalizeDomain(raw)
	if err != nil {
		return Rule{}, fmt.Errorf("%w: enter a valid domain such as corp.example", ErrInvalid)
	}
	rule.Domain = domain
	rule.Description = strings.TrimSpace(rule.Description)
	if len(rule.Description) > maxDescription || strings.ContainsAny(rule.Description, "\r\n\x00") {
		return Rule{}, fmt.Errorf("%w: description must be a single line of at most %d characters", ErrInvalid, maxDescription)
	}
	if len(rule.Upstreams) == 0 || len(rule.Upstreams) > MaxUpstreams {
		return Rule{}, fmt.Errorf("%w: configure between 1 and %d upstream resolvers", ErrInvalid, MaxUpstreams)
	}
	upstreams := make([]string, 0, len(rule.Upstreams))
	for _, raw := range rule.Upstreams {
		address, err := ParseUpstream(raw)
		if err != nil {
			return Rule{}, err
		}
		if s.isSelf(address) {
			return Rule{}, fmt.Errorf("%w: %s is this resolver's own listener and would create a forwarding loop", ErrInvalid, address)
		}
		value := address.String()
		if slices.Contains(upstreams, value) {
			return Rule{}, fmt.Errorf("%w: %s is listed twice", ErrInvalid, value)
		}
		upstreams = append(upstreams, value)
	}
	rule.Upstreams = upstreams
	return rule, nil
}

// ParseUpstream accepts an IP address with an optional port (default 53).
// Hostnames are rejected because resolving them would itself need DNS.
func ParseUpstream(raw string) (netip.AddrPort, error) {
	raw = strings.TrimSpace(raw)
	address, err := netip.ParseAddrPort(raw)
	if err != nil {
		ip, ipErr := netip.ParseAddr(strings.Trim(raw, "[]"))
		if ipErr != nil {
			return netip.AddrPort{}, fmt.Errorf("%w: %q is not an IP address or IP:port", ErrInvalid, raw)
		}
		address = netip.AddrPortFrom(ip, defaultUpstreamPort)
	}
	ip := address.Addr().Unmap()
	if ip.Zone() != "" || !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() || address.Port() == 0 {
		return netip.AddrPort{}, fmt.Errorf("%w: %q is not a usable resolver address", ErrInvalid, raw)
	}
	return netip.AddrPortFrom(ip, address.Port()), nil
}

func (s *Service) isSelf(address netip.AddrPort) bool {
	for _, listen := range s.self {
		if listen.Port() != address.Port() {
			continue
		}
		if listen.Addr().Unmap() == address.Addr() || (listen.Addr().IsUnspecified() && address.Addr().IsLoopback()) {
			return true
		}
	}
	return false
}

// TestResult reports a diagnostic query sent through one rule.
type TestResult struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Rcode      string   `json:"rcode,omitempty"`
	Answers    []string `json:"answers"`
	Upstream   string   `json:"upstream,omitempty"`
	DurationMs float64  `json:"duration_ms"`
	Error      string   `json:"error,omitempty"`
}

// Test resolves name through the rule's upstreams only, bypassing the cache
// and health cooldown. The name must lie within the rule's domain.
func (s *Service) Test(ctx context.Context, id int64, name string, qtype uint16) (TestResult, error) {
	s.mu.Lock()
	index := slices.IndexFunc(s.rules, func(rule Rule) bool { return rule.ID == id })
	var rule Rule
	if index >= 0 {
		rule = s.rules[index]
	}
	s.mu.Unlock()
	if index < 0 {
		return TestResult{}, ErrNotFound
	}
	if strings.TrimSpace(name) == "" {
		name = rule.Domain
	}
	normalized, err := filtering.NormalizeDomain(name)
	if err != nil || strings.HasPrefix(strings.TrimSpace(name), "*") {
		return TestResult{}, fmt.Errorf("%w: enter a valid name to test", ErrInvalid)
	}
	if normalized != rule.Domain && !strings.HasSuffix(normalized, "."+rule.Domain) {
		return TestResult{}, fmt.Errorf("%w: the test name must be %s or a name below it", ErrInvalid, rule.Domain)
	}
	if qtype == 0 {
		qtype = wire.TypeA
	}
	q := new(wire.Msg)
	q.SetQuestion(wire.Fqdn(normalized), qtype)
	q.RecursionDesired = true
	result := TestResult{Name: q.Question[0].Name, Type: wire.TypeToString[qtype], Answers: []string{}}
	started := time.Now()
	testCtx, cancel := context.WithTimeout(ctx, s.options.Timeout*time.Duration(len(rule.Upstreams)+1))
	defer cancel()
	response, upstream, err := s.forwarder(rule.Upstreams, nil).Resolve(testCtx, q)
	result.DurationMs = float64(time.Since(started)) / float64(time.Millisecond)
	if err != nil {
		result.Error = "No configured upstream answered: " + err.Error()
		return result, nil
	}
	result.Rcode = wire.RcodeToString[response.Rcode]
	result.Upstream = upstream
	for _, rr := range response.Answer {
		result.Answers = append(result.Answers, strings.TrimSpace(strings.TrimPrefix(rr.String(), rr.Header().String())))
	}
	return result, nil
}

// Router sends matching queries to their rule's upstreams and everything
// else to Default. It implements dns.Upstream.
type Router struct {
	Rules   *Service
	Default dns.Upstream
}

func (r *Router) Resolve(ctx context.Context, q *wire.Msg) (*wire.Msg, string, error) {
	if r.Rules != nil && len(q.Question) == 1 {
		if route := r.Rules.match(q.Question[0].Name); route != nil {
			return route.forwarder.Resolve(ctx, q)
		}
	}
	return r.Default.Resolve(ctx, q)
}
