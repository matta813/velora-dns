package filtering

import (
	"context"
	"github.com/matta813/velora-dns/internal/database"
	"net"
	"sync/atomic"
	"time"
)

type DomainStore interface {
	LoadBlocklistDomains(context.Context) ([]string, error)
}
type SourceStore interface {
	DomainStore
	ListBlocklistSources(context.Context) ([]database.BlocklistSource, error)
	GetBlocklistSource(context.Context, int64) (database.BlocklistSource, error)
	CreateBlocklistSource(context.Context, string, string, bool) (database.BlocklistSource, error)
	ReplaceBlocklistDomains(context.Context, int64, []string, time.Time) error
	RecordBlocklistError(context.Context, int64, string) error
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
func (s *Service) List(ctx context.Context) ([]database.BlocklistSource, error) {
	return s.store.(SourceStore).ListBlocklistSources(ctx)
}
func (s *Service) Create(ctx context.Context, name, url string, enabled bool) (database.BlocklistSource, error) {
	return s.store.(SourceStore).CreateBlocklistSource(ctx, name, url, enabled)
}
func (s *Service) Refresh(ctx context.Context, id int64) (database.BlocklistSource, error) {
	store := s.store.(SourceStore)
	source, err := store.GetBlocklistSource(ctx, id)
	if err != nil {
		return source, err
	}
	domains, err := FetchHosts(ctx, source.URL, nil, net.DefaultResolver.LookupIPAddr)
	if err != nil {
		_ = store.RecordBlocklistError(ctx, id, err.Error())
		return source, err
	}
	if err = store.ReplaceBlocklistDomains(ctx, id, domains, time.Now()); err != nil {
		return source, err
	}
	if err = s.Reload(ctx); err != nil {
		return source, err
	}
	return store.GetBlocklistSource(ctx, id)
}
