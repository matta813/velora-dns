package filtering

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Service struct {
	store   SourceStore
	static  []Rule
	current atomic.Pointer[Matcher]
	mu      sync.Mutex
	sources []Source
	fetch   func(context.Context, string) ([]string, error)
}

func NewService(ctx context.Context, store SourceStore, rules []Rule) (*Service, error) {
	s := &Service{store: store, static: append([]Rule{}, rules...), fetch: FetchHosts}
	sources, err := store.LoadSources(ctx)
	if err != nil {
		return nil, err
	}
	matcher, err := s.compile(sources)
	if err != nil {
		return nil, err
	}
	s.sources = sources
	s.current.Store(matcher)
	return s, nil
}
func (s *Service) compile(sources []Source) (*Matcher, error) {
	if len(sources) > MaxSources {
		return nil, fmt.Errorf("%w: too many sources", ErrInvalid)
	}
	rules := append([]Rule{}, s.static...)
	total := 0
	for _, source := range sources {
		total += len(source.Domains)
		if len(source.Domains) > MaxDomains || total > MaxTotalDomains {
			return nil, fmt.Errorf("%w: too many stored domains", ErrInvalid)
		}
		if source.Enabled {
			for _, domain := range source.Domains {
				rules = append(rules, Rule{Domain: domain, Wildcard: true, Action: Block})
			}
		}
	}
	return New(rules)
}
func (s *Service) Blocked(domain string) bool { return s.current.Load().Blocked(domain) }
func (s *Service) List(context.Context) ([]Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Source{}, s.sources...)
	for i := range out {
		out[i].DomainCount = len(out[i].Domains)
		out[i].Domains = nil
		if out[i].LastUpdatedAt != nil {
			timestamp := *out[i].LastUpdatedAt
			out[i].LastUpdatedAt = &timestamp
		}
	}
	return out, nil
}
func (s *Service) Create(ctx context.Context, name, url string, enabled bool) (Source, error) {
	if !s.mu.TryLock() {
		return Source{}, ErrBusy
	}
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	if len(name) < 1 || len(name) > 120 || strings.ContainsAny(name, "\r\n\x00") {
		return Source{}, ErrInvalid
	}
	if url != "" {
		if err := ValidateSourceURL(url); err != nil {
			return Source{}, err
		}
	}
	for _, source := range s.sources {
		if source.Name == name {
			return Source{}, ErrExists
		}
	}
	return s.save(ctx, Source{Name: name, URL: url, Enabled: enabled})
}
func (s *Service) index(id int64) int {
	for i, source := range s.sources {
		if source.ID == id {
			return i
		}
	}
	return -1
}
func (s *Service) save(ctx context.Context, source Source) (Source, error) {
	candidate := append([]Source{}, s.sources...)
	index := s.index(source.ID)
	if index < 0 {
		candidate = append(candidate, source)
	} else {
		candidate[index] = source
	}
	matcher, err := s.compile(candidate)
	if err != nil {
		return Source{}, err
	}
	saved, err := s.store.SaveSource(ctx, source)
	if err != nil {
		return Source{}, err
	}
	saved.DomainCount = len(saved.Domains)
	if index < 0 {
		candidate[len(candidate)-1] = saved
	} else {
		candidate[index] = saved
	}
	s.sources = candidate
	s.current.Store(matcher)
	return saved, nil
}
func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) (Source, error) {
	if !s.mu.TryLock() {
		return Source{}, ErrBusy
	}
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 {
		return Source{}, ErrNotFound
	}
	source := s.sources[i]
	source.Enabled = enabled
	return s.save(ctx, source)
}
func (s *Service) ReplaceLocal(ctx context.Context, id int64, text string) (Source, error) {
	domains, err := ParseHosts([]byte(text))
	if err != nil {
		return Source{}, err
	}
	if !s.mu.TryLock() {
		return Source{}, ErrBusy
	}
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 {
		return Source{}, ErrNotFound
	}
	source := s.sources[i]
	if source.URL != "" {
		return Source{}, fmt.Errorf("%w: manual content requires a local source", ErrInvalid)
	}
	now := time.Now().UTC()
	source.Domains = domains
	source.LastUpdatedAt = &now
	source.LastError = ""
	return s.save(ctx, source)
}
func (s *Service) Refresh(ctx context.Context, id int64) (Source, error) {
	if !s.mu.TryLock() {
		return Source{}, ErrBusy
	}
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 {
		return Source{}, ErrNotFound
	}
	source := s.sources[i]
	if source.URL == "" {
		return Source{}, fmt.Errorf("%w: local sources use manual content", ErrInvalid)
	}
	domains, err := s.fetch(ctx, source.URL)
	if err != nil {
		source.LastError = "Download or list validation failed; previous domains retained."
		if _, saveErr := s.save(ctx, source); saveErr != nil {
			return Source{}, fmt.Errorf("record update failure: %w", saveErr)
		}
		return source, fmt.Errorf("source update failed")
	}
	now := time.Now().UTC()
	source.Domains = domains
	source.LastUpdatedAt = &now
	source.LastError = ""
	return s.save(ctx, source)
}
func (s *Service) Delete(ctx context.Context, id int64) error {
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 {
		return ErrNotFound
	}
	candidate := append([]Source{}, s.sources[:i]...)
	candidate = append(candidate, s.sources[i+1:]...)
	matcher, err := s.compile(candidate)
	if err != nil {
		return err
	}
	if err = s.store.DeleteSource(ctx, id); err != nil {
		return err
	}
	s.sources = candidate
	s.current.Store(matcher)
	return nil
}
