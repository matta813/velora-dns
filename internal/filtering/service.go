package filtering

import (
	"context"
	"sync/atomic"
)

type DomainStore interface {
	LoadBlocklistDomains(context.Context) ([]string, error)
}
type Service struct {
	store   DomainStore
	static  []Rule
	current atomic.Pointer[Matcher]
}

func NewService(ctx context.Context, store DomainStore, rules []Rule) (*Service, error) {
	s := &Service{store: store, static: append([]Rule{}, rules...)}
	if err := s.Reload(ctx); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Service) Reload(ctx context.Context) error {
	domains, err := s.store.LoadBlocklistDomains(ctx)
	if err != nil {
		return err
	}
	rules := append([]Rule{}, s.static...)
	for _, domain := range domains {
		rules = append(rules, Rule{Domain: domain, Wildcard: true, Action: Block})
	}
	matcher, err := New(rules)
	if err != nil {
		return err
	}
	s.current.Store(matcher)
	return nil
}
func (s *Service) Blocked(domain string) bool {
	matcher := s.current.Load()
	return matcher != nil && matcher.Blocked(domain)
}
