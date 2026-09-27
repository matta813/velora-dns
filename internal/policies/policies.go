// Package policies applies per-client filtering: a known client can use the
// global filtering, bypass it entirely, or use its own set of blocklists
// plus extra allowed and blocked domains.
package policies

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/filtering"
)

const (
	ModeDefault  = "default"
	ModeDisabled = "disabled"
	ModeCustom   = "custom"

	MaxPolicies     = 64
	MaxExtraDomains = 1000
)

var (
	ErrInvalid  = errors.New("invalid policy")
	ErrNotFound = errors.New("policy not found")
	ErrExists   = errors.New("policy already exists")
)

// Policy decides how DNS filtering applies to one client. Blocklists, Allow
// and Block are only used in custom mode.
type Policy struct {
	ID         int64    `json:"id"`
	ClientID   int64    `json:"client_id"`
	Mode       string   `json:"mode"`
	Blocklists []int64  `json:"blocklists"`
	Allow      []string `json:"allow"`
	Block      []string `json:"block"`
	Enabled    bool     `json:"enabled"`
}

type Store interface {
	LoadPolicies(context.Context) ([]Policy, error)
	SavePolicy(context.Context, Policy) (Policy, error)
	DeletePolicy(context.Context, int64) error
}

// Global is the filtering service: the default policy and the source of
// blocklist domains for custom policies.
type Global interface {
	Blocked(string) bool
	Subset(sourceIDs []int64, extra []filtering.Rule) (*filtering.Matcher, error)
}

// Clients resolves a query's source address to a known client.
type Clients interface {
	Lookup(netip.Addr) (clients.Client, bool)
}

// compiled is what the DNS path reads: nil matcher means "no filtering",
// and a client without an entry uses the global filter.
type compiled struct {
	byClient map[int64]*filtering.Matcher
}

// Effective explains which policy applies to an address.
type Effective struct {
	Client *clients.Client `json:"client"`
	Policy *Policy         `json:"policy"`
	Mode   string          `json:"mode"`
}

type Service struct {
	store    Store
	global   Global
	clients  Clients
	mu       sync.Mutex
	policies []Policy
	current  atomic.Pointer[compiled]
	// rebuild serializes background recompilation after blocklist changes.
	rebuild sync.Mutex
}

func NewService(ctx context.Context, store Store, global Global, known Clients) (*Service, error) {
	policies, err := store.LoadPolicies(ctx)
	if err != nil {
		return nil, err
	}
	s := &Service{store: store, global: global, clients: known, policies: policies}
	next, err := s.compile(policies)
	if err != nil {
		return nil, err
	}
	s.current.Store(next)
	return s, nil
}

func (s *Service) compile(policies []Policy) (*compiled, error) {
	next := &compiled{byClient: map[int64]*filtering.Matcher{}}
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		switch policy.Mode {
		case ModeDisabled:
			next.byClient[policy.ClientID] = nil
		case ModeCustom:
			rules := make([]filtering.Rule, 0, len(policy.Allow)+len(policy.Block))
			for _, domain := range policy.Block {
				rules = append(rules, filtering.Rule{Domain: domain, Wildcard: true, Action: filtering.Block})
			}
			for _, domain := range policy.Allow {
				rules = append(rules, filtering.Rule{Domain: domain, Wildcard: true, Action: filtering.Allow})
			}
			matcher, err := s.global.Subset(policy.Blocklists, rules)
			if err != nil {
				return nil, err
			}
			next.byClient[policy.ClientID] = matcher
		}
	}
	return next, nil
}

// Recompile rebuilds custom policies from the current blocklist contents.
// The filtering service calls it after sources change.
func (s *Service) Recompile() {
	s.rebuild.Lock()
	defer s.rebuild.Unlock()
	s.mu.Lock()
	policies := slices.Clone(s.policies)
	s.mu.Unlock()
	if next, err := s.compile(policies); err == nil {
		s.current.Store(next)
	}
}

// Blocked applies the global policy.
func (s *Service) Blocked(name string) bool { return s.global.Blocked(name) }

// BlockedFor applies the policy of the client that owns addr.
func (s *Service) BlockedFor(addr netip.Addr, name string) bool {
	current := s.current.Load()
	if len(current.byClient) > 0 {
		if client, ok := s.clients.Lookup(addr); ok {
			if matcher, found := current.byClient[client.ID]; found {
				return matcher != nil && matcher.Blocked(name)
			}
		}
	}
	return s.global.Blocked(name)
}

// Effective reports the client and policy used for addr.
func (s *Service) Effective(addr netip.Addr) Effective {
	out := Effective{Mode: ModeDefault}
	client, ok := s.clients.Lookup(addr)
	if !ok {
		return out
	}
	out.Client = &client
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, policy := range s.policies {
		if policy.ClientID == client.ID && policy.Enabled {
			policy := policy
			out.Policy = &policy
			out.Mode = policy.Mode
		}
	}
	return out
}

func (s *Service) List(context.Context) ([]Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.policies), nil
}

func (s *Service) Create(ctx context.Context, policy Policy) (Policy, error) {
	policy.ID = 0
	return s.save(ctx, policy)
}

func (s *Service) Update(ctx context.Context, id int64, policy Policy) (Policy, error) {
	if id < 1 {
		return Policy{}, ErrNotFound
	}
	policy.ID = id
	return s.save(ctx, policy)
}

func (s *Service) save(ctx context.Context, policy Policy) (Policy, error) {
	normalized, err := normalize(policy)
	if err != nil {
		return Policy{}, err
	}
	s.rebuild.Lock()
	defer s.rebuild.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	if normalized.ID != 0 {
		if index = slices.IndexFunc(s.policies, func(existing Policy) bool { return existing.ID == normalized.ID }); index < 0 {
			return Policy{}, ErrNotFound
		}
	}
	for i, existing := range s.policies {
		if i != index && existing.ClientID == normalized.ClientID {
			return Policy{}, fmt.Errorf("%w: this client already has a policy", ErrExists)
		}
	}
	if normalized.ID == 0 && len(s.policies) >= MaxPolicies {
		return Policy{}, fmt.Errorf("%w: at most %d policies", ErrInvalid, MaxPolicies)
	}
	candidate := slices.Clone(s.policies)
	if index < 0 {
		candidate = append(candidate, normalized)
	} else {
		candidate[index] = normalized
	}
	next, err := s.compile(candidate)
	if err != nil {
		return Policy{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	saved, err := s.store.SavePolicy(ctx, normalized)
	if err != nil {
		return Policy{}, err
	}
	if index < 0 {
		candidate[len(candidate)-1] = saved
	} else {
		candidate[index] = saved
	}
	sort.SliceStable(candidate, func(i, j int) bool { return candidate[i].ClientID < candidate[j].ClientID })
	s.policies = candidate
	s.current.Store(next)
	return saved, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	s.rebuild.Lock()
	defer s.rebuild.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.policies, func(policy Policy) bool { return policy.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	if err := s.store.DeletePolicy(ctx, id); err != nil {
		return err
	}
	candidate := slices.Delete(slices.Clone(s.policies), i, i+1)
	s.policies = candidate
	if next, err := s.compile(candidate); err == nil {
		s.current.Store(next)
	}
	return nil
}

// ForgetClient drops the policy of a deleted client; the database removes
// the row through its foreign key.
func (s *Service) ForgetClient(clientID int64) {
	s.rebuild.Lock()
	defer s.rebuild.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	candidate := slices.DeleteFunc(slices.Clone(s.policies), func(policy Policy) bool { return policy.ClientID == clientID })
	if len(candidate) == len(s.policies) {
		return
	}
	s.policies = candidate
	if next, err := s.compile(candidate); err == nil {
		s.current.Store(next)
	}
}

func normalize(policy Policy) (Policy, error) {
	if policy.ClientID < 1 {
		return Policy{}, fmt.Errorf("%w: choose a client", ErrInvalid)
	}
	switch policy.Mode {
	case ModeDefault, ModeDisabled:
		policy.Blocklists, policy.Allow, policy.Block = []int64{}, []string{}, []string{}
		return policy, nil
	case ModeCustom:
	default:
		return Policy{}, fmt.Errorf("%w: mode must be default, disabled or custom", ErrInvalid)
	}
	blocklists := []int64{}
	for _, id := range policy.Blocklists {
		if id < 1 {
			return Policy{}, fmt.Errorf("%w: unknown blocklist %d", ErrInvalid, id)
		}
		if !slices.Contains(blocklists, id) {
			blocklists = append(blocklists, id)
		}
	}
	slices.Sort(blocklists)
	policy.Blocklists = blocklists
	var err error
	if policy.Allow, err = domains(policy.Allow, "allowed"); err != nil {
		return Policy{}, err
	}
	if policy.Block, err = domains(policy.Block, "blocked"); err != nil {
		return Policy{}, err
	}
	if len(policy.Allow)+len(policy.Block) > MaxExtraDomains {
		return Policy{}, fmt.Errorf("%w: at most %d extra domains", ErrInvalid, MaxExtraDomains)
	}
	return policy, nil
}

func domains(values []string, label string) ([]string, error) {
	out := []string{}
	for _, raw := range values {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		domain, err := filtering.NormalizeDomain(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not a valid %s domain", ErrInvalid, strings.TrimSpace(raw), label)
		}
		if !slices.Contains(out, domain) {
			out = append(out, domain)
		}
	}
	return out, nil
}
