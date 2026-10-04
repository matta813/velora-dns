// Package rewrites answers selected names with fixed A, AAAA or CNAME data
// without creating a full authoritative zone.
package rewrites

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/filtering"
	wire "github.com/miekg/dns"
)

const (
	MaxRules       = 2048
	MaxPerName     = 16
	maxDescription = 200
	answerTTL      = 300
)

var (
	ErrInvalid  = errors.New("invalid rewrite")
	ErrNotFound = errors.New("rewrite not found")
	ErrExists   = errors.New("an identical rewrite already exists")
	ErrConflict = errors.New("conflicting rewrite")
)

// Rule answers queries for Name. A Name starting with "*." matches every
// name below the parent domain (but not the parent itself).
type Rule struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Value       string `json:"value"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

// RuleStatus adds hints about other features that affect a rule.
type RuleStatus struct {
	Rule
	// BlockedBy is "blocklist" when filtering blocks the name first.
	BlockedBy string `json:"blocked_by,omitempty"`
	// OverridesZone names a local zone whose answers this rule replaces.
	OverridesZone string `json:"overrides_zone,omitempty"`
}

type Store interface {
	LoadRewrites(context.Context) ([]Rule, error)
	SaveRewrite(context.Context, Rule) (Rule, error)
	DeleteRewrite(context.Context, int64) error
}

// Hints lets the service explain precedence to administrators.
type Hints struct {
	Blocked func(name string) bool
	ZoneFor func(name string) (string, bool)
	// OnChange is called with an affected name so cached answers can be dropped.
	OnChange func(name string)
}

type table struct {
	exact    map[string][]Rule
	wildcard map[string][]Rule // keyed by parent domain
}

type Service struct {
	store   Store
	hints   Hints
	mu      sync.Mutex
	rules   []Rule
	current atomic.Pointer[table]
}

func NewService(ctx context.Context, store Store, hints Hints) (*Service, error) {
	rules, err := store.LoadRewrites(ctx)
	if err != nil {
		return nil, err
	}
	s := &Service{store: store, hints: hints, rules: rules}
	s.current.Store(compile(rules))
	return s, nil
}

func compile(rules []Rule) *table {
	t := &table{exact: map[string][]Rule{}, wildcard: map[string][]Rule{}}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if parent, ok := strings.CutPrefix(rule.Name, "*."); ok {
			t.wildcard[parent] = append(t.wildcard[parent], rule)
		} else {
			t.exact[rule.Name] = append(t.exact[rule.Name], rule)
		}
	}
	return t
}

// match returns the enabled rules for name: an exact match beats any
// wildcard, and the wildcard with the longest parent wins.
func (t *table) match(name string) []Rule {
	if rules, ok := t.exact[name]; ok {
		return rules
	}
	for parent := name; ; {
		_, next, found := strings.Cut(parent, ".")
		if !found {
			return nil
		}
		if rules, ok := t.wildcard[next]; ok {
			return rules
		}
		parent = next
	}
}

// Rewrite builds an answer for q when an enabled rule covers its name.
// Names with only address rewrites return NODATA for other record types,
// so a partial override never leaks to upstream resolvers.
func (s *Service) Rewrite(q *wire.Msg) (*wire.Msg, bool) {
	m, _, ok := s.rewrite(q)
	return m, ok
}

// rewrite is Rewrite plus the enabled rules that covered the name.
func (s *Service) rewrite(q *wire.Msg) (*wire.Msg, []Rule, bool) {
	if len(q.Question) != 1 || q.Question[0].Qclass != wire.ClassINET {
		return nil, nil, false
	}
	question := q.Question[0]
	rules := s.current.Load().match(strings.TrimSuffix(strings.ToLower(question.Name), "."))
	if len(rules) == 0 {
		return nil, nil, false
	}
	m := new(wire.Msg)
	m.SetReply(q)
	m.RecursionAvailable = true
	header := func(rrtype uint16) wire.RR_Header {
		return wire.RR_Header{Name: question.Name, Rrtype: rrtype, Class: wire.ClassINET, Ttl: answerTTL}
	}
	for _, rule := range rules {
		switch rule.Type {
		case "CNAME":
			m.Answer = []wire.RR{&wire.CNAME{Hdr: header(wire.TypeCNAME), Target: wire.Fqdn(rule.Value)}}
			return m, rules, true
		case "A":
			if question.Qtype == wire.TypeA {
				m.Answer = append(m.Answer, &wire.A{Hdr: header(wire.TypeA), A: netip.MustParseAddr(rule.Value).AsSlice()})
			}
		case "AAAA":
			if question.Qtype == wire.TypeAAAA {
				m.Answer = append(m.Answer, &wire.AAAA{Hdr: header(wire.TypeAAAA), AAAA: netip.MustParseAddr(rule.Value).AsSlice()})
			}
		}
	}
	return m, rules, true
}

func (s *Service) List(context.Context) ([]RuleStatus, error) {
	s.mu.Lock()
	rules := slices.Clone(s.rules)
	s.mu.Unlock()
	out := make([]RuleStatus, 0, len(rules))
	for _, rule := range rules {
		status := RuleStatus{Rule: rule}
		probe := strings.TrimPrefix(rule.Name, "*.")
		if strings.HasPrefix(rule.Name, "*.") {
			probe = "x." + probe
		}
		if s.hints.Blocked != nil && s.hints.Blocked(probe) {
			status.BlockedBy = "blocklist"
		}
		if s.hints.ZoneFor != nil {
			if zone, ok := s.hints.ZoneFor(probe); ok {
				status.OverridesZone = strings.TrimSuffix(zone, ".")
			}
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
	normalized, err := normalize(rule)
	if err != nil {
		return Rule{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	sameName := 0
	for i, existing := range s.rules {
		if existing.ID == normalized.ID && normalized.ID != 0 {
			index = i
			continue
		}
		if existing.Name != normalized.Name {
			continue
		}
		if existing.Type == normalized.Type && existing.Value == normalized.Value {
			return Rule{}, ErrExists
		}
		if existing.Type == "CNAME" || normalized.Type == "CNAME" {
			return Rule{}, fmt.Errorf("%w: a CNAME rewrite cannot share its name with other rewrites", ErrConflict)
		}
		sameName++
	}
	if normalized.ID != 0 && index < 0 {
		return Rule{}, ErrNotFound
	}
	if sameName >= MaxPerName {
		return Rule{}, fmt.Errorf("%w: at most %d rewrites per name", ErrInvalid, MaxPerName)
	}
	if normalized.ID == 0 && len(s.rules) >= MaxRules {
		return Rule{}, fmt.Errorf("%w: at most %d rewrites", ErrInvalid, MaxRules)
	}
	saved, err := s.store.SaveRewrite(ctx, normalized)
	if err != nil {
		return Rule{}, err
	}
	candidate := slices.Clone(s.rules)
	oldName := ""
	if index < 0 {
		candidate = append(candidate, saved)
	} else {
		oldName = candidate[index].Name
		candidate[index] = saved
	}
	s.rules = candidate
	s.current.Store(compile(candidate))
	s.changed(saved.Name)
	if oldName != "" && oldName != saved.Name {
		s.changed(oldName)
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
	if err := s.store.DeleteRewrite(ctx, id); err != nil {
		return err
	}
	name := s.rules[index].Name
	s.rules = slices.Delete(slices.Clone(s.rules), index, index+1)
	s.current.Store(compile(s.rules))
	s.changed(name)
	return nil
}

// changed reports the affected name; wildcards report their parent so that
// every cached name below it can be invalidated.
func (s *Service) changed(name string) {
	if s.hints.OnChange != nil {
		s.hints.OnChange(strings.TrimPrefix(name, "*."))
	}
}

func normalize(rule Rule) (Rule, error) {
	raw := strings.TrimSpace(rule.Name)
	wildcard := strings.HasPrefix(raw, "*.")
	if strings.Contains(strings.TrimPrefix(raw, "*."), "*") {
		return Rule{}, fmt.Errorf("%w: a wildcard is only allowed as the first label, like *.example.home", ErrInvalid)
	}
	name, err := filtering.NormalizeDomain(raw)
	if err != nil {
		return Rule{}, fmt.Errorf("%w: enter a valid name such as nas.home or *.lab.home", ErrInvalid)
	}
	if wildcard {
		if !strings.Contains(name, ".") {
			return Rule{}, fmt.Errorf("%w: wildcards need a parent domain with at least two labels", ErrInvalid)
		}
		name = "*." + name
	}
	rule.Name = name
	rule.Type = strings.ToUpper(strings.TrimSpace(rule.Type))
	value := strings.TrimSpace(rule.Value)
	switch rule.Type {
	case "A", "AAAA":
		ip, err := netip.ParseAddr(value)
		if err != nil || ip.Zone() != "" || (rule.Type == "A") != ip.Is4() {
			return Rule{}, fmt.Errorf("%w: %s rewrites need an IPv%s address", ErrInvalid, rule.Type, map[string]string{"A": "4", "AAAA": "6"}[rule.Type])
		}
		rule.Value = ip.String()
	case "CNAME":
		target, err := filtering.NormalizeDomain(value)
		if err != nil || strings.HasPrefix(value, "*") {
			return Rule{}, fmt.Errorf("%w: CNAME rewrites need a target host name", ErrInvalid)
		}
		if target == strings.TrimPrefix(name, "*.") && !wildcard {
			return Rule{}, fmt.Errorf("%w: a CNAME cannot point to itself", ErrInvalid)
		}
		rule.Value = target
	default:
		return Rule{}, fmt.Errorf("%w: type must be A, AAAA or CNAME", ErrInvalid)
	}
	rule.Description = strings.TrimSpace(rule.Description)
	if len(rule.Description) > maxDescription || strings.ContainsAny(rule.Description, "\r\n\x00") {
		return Rule{}, fmt.Errorf("%w: description must be a single line of at most %d characters", ErrInvalid, maxDescription)
	}
	return rule, nil
}

// Local is the resolver's local-answer layer: enabled rewrites answer
// first, then the wrapped authoritative zones.
type Local struct {
	Rewrites *Service
	Next     interface {
		Lookup(*wire.Msg) (*wire.Msg, bool)
	}
}

func (l *Local) Lookup(q *wire.Msg) (*wire.Msg, bool) {
	match, ok := l.lookup(q)
	return match.Msg, ok
}

// ExplainLocal is Lookup that also reports which layer and rules answered.
func (l *Local) ExplainLocal(q *wire.Msg) (dns.LocalMatch, bool) { return l.lookup(q) }

func (l *Local) lookup(q *wire.Msg) (dns.LocalMatch, bool) {
	if l.Rewrites != nil {
		if m, rules, ok := l.Rewrites.rewrite(q); ok {
			match := dns.LocalMatch{Msg: m, Stage: "rewrite"}
			for _, rule := range rules {
				match.Rules = append(match.Rules, dns.RuleRef{ID: rule.ID, Name: rule.Name, Detail: rule.Type + " " + rule.Value})
			}
			return match, true
		}
	}
	if l.Next == nil {
		return dns.LocalMatch{}, false
	}
	m, ok := l.Next.Lookup(q)
	if !ok {
		return dns.LocalMatch{}, false
	}
	match := dns.LocalMatch{Msg: m, Stage: "zone"}
	if z, yes := l.Next.(interface{ ZoneFor(string) (string, bool) }); yes && len(q.Question) == 1 {
		if name, found := z.ZoneFor(q.Question[0].Name); found {
			match.Rules = []dns.RuleRef{{Name: strings.TrimSuffix(name, ".")}}
		}
	}
	return match, true
}
