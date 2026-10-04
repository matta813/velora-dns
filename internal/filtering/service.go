package filtering

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Service struct {
	store     SourceStore
	static    []Rule
	current   atomic.Pointer[Matcher]
	mu        sync.Mutex
	sources   []Source
	fetch     func(context.Context, string) ([]string, error)
	now       func() time.Time
	onRefresh func(source Source, previousFailures int, err error)
	// published mirrors sources for lock-free readers such as per-client
	// policies; onChange is told (asynchronously) whenever it changes.
	published atomic.Pointer[[]Source]
	onChange  atomic.Pointer[func()]
}

// Retry delays after a failed refresh grow from retryBase and never exceed
// the source's own interval, so a broken list is retried sooner than its
// normal schedule without hammering the remote server.
const retryBase = 5 * time.Minute

func NewService(ctx context.Context, store SourceStore, rules []Rule) (*Service, error) {
	s := &Service{store: store, static: append([]Rule{}, rules...), fetch: FetchHosts, now: time.Now}
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
	s.publish()
	return s, nil
}

// publish exposes the current sources to lock-free readers and notifies the
// change observer without holding the service lock.
func (s *Service) publish() {
	snapshot := append([]Source{}, s.sources...)
	s.published.Store(&snapshot)
	if observer := s.onChange.Load(); observer != nil {
		go (*observer)()
	}
}

// SetChangeObserver registers fn to run (in its own goroutine) after any
// source is added, changed, refreshed or removed.
func (s *Service) SetChangeObserver(fn func()) { s.onChange.Store(&fn) }

// StaticRules returns the configuration-defined allow and block rules.
func (s *Service) StaticRules() []Rule { return append([]Rule{}, s.static...) }

// Subset compiles a matcher from the static rules, extra, and the domains of
// the listed sources. A listed source applies even when it is disabled
// globally, so a list can be reserved for specific clients. It never takes
// the service lock and is safe to call from the DNS path.
func (s *Service) Subset(sourceIDs []int64, extra []Rule) (*Matcher, error) {
	rules := append(s.StaticRules(), extra...)
	if published := s.published.Load(); published != nil {
		for _, source := range *published {
			for _, id := range sourceIDs {
				if source.ID == id {
					for _, domain := range source.Domains {
						rules = append(rules, Rule{Domain: domain, Wildcard: true, Action: Block})
					}
					break
				}
			}
		}
	}
	return New(rules)
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

// SourceRef names a blocklist source.
type SourceRef struct {
	ID   int64
	Name string
}

// Attribution labels the parts of the filter that cover a name; it never
// decides anything, the matchers do. Static is the configuration allow/block
// rule action (when StaticMatched) and Sources the lists containing the name.
type Attribution struct {
	StaticMatched bool
	Static        Action
	Sources       []SourceRef
}

// Attribute reports which configuration rules and blocklist sources cover
// name. sourceIDs selects sources the way Subset does; nil means every
// enabled source. It only reads the published snapshot.
func (s *Service) Attribute(name string, sourceIDs []int64) Attribution {
	var out Attribution
	n, err := NormalizeDomain(name)
	if err != nil {
		return out
	}
	if static, err := New(s.static); err == nil {
		out.Static, out.StaticMatched = static.Match(n)
	}
	covers := map[string]bool{}
	for current := n; ; {
		covers[current] = true
		i := strings.IndexByte(current, '.')
		if i < 0 {
			break
		}
		current = current[i+1:]
	}
	published := s.published.Load()
	if published == nil {
		return out
	}
	for _, source := range *published {
		if sourceIDs == nil && !source.Enabled || sourceIDs != nil && !slices.Contains(sourceIDs, source.ID) {
			continue
		}
		for _, domain := range source.Domains {
			if covers[strings.TrimPrefix(domain, "*.")] {
				out.Sources = append(out.Sources, SourceRef{ID: source.ID, Name: source.Name})
				break
			}
		}
	}
	return out
}
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
		if out[i].LastAttemptAt != nil {
			timestamp := *out[i].LastAttemptAt
			out[i].LastAttemptAt = &timestamp
		}
		out[i].NextUpdateAt = s.nextUpdate(out[i])
	}
	return out, nil
}

// SetRefreshObserver registers a callback for every completed refresh
// attempt. previousFailures lets callers detect recovery after failures.
func (s *Service) SetRefreshObserver(observer func(source Source, previousFailures int, err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRefresh = observer
}

// nextUpdate returns when a scheduled refresh is due, or nil when the source
// is not scheduled. A source that was never attempted is due immediately.
func (s *Service) nextUpdate(source Source) *time.Time {
	if source.URL == "" || !source.Enabled || source.UpdateInterval == 0 {
		return nil
	}
	if source.LastAttemptAt == nil {
		now := s.now().UTC()
		return &now
	}
	delay := time.Duration(source.UpdateInterval) * time.Second
	if source.ConsecutiveFailures > 0 {
		backoff := retryBase << min(source.ConsecutiveFailures-1, 10)
		delay = min(backoff, delay)
	}
	next := source.LastAttemptAt.Add(delay).UTC()
	return &next
}
func (s *Service) Create(ctx context.Context, name, url string, enabled bool, interval int) (Source, error) {
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
	if !ValidInterval(interval) || (url == "" && interval != 0) {
		return Source{}, fmt.Errorf("%w: schedule must be manual or between 1 hour and 7 days, and only applies to URL sources", ErrInvalid)
	}
	for _, source := range s.sources {
		if source.Name == name {
			return Source{}, ErrExists
		}
	}
	return s.save(ctx, Source{Name: name, URL: url, Enabled: enabled, UpdateInterval: interval})
}

// SetSchedule changes how often a URL source is refreshed automatically.
func (s *Service) SetSchedule(ctx context.Context, id int64, interval int) (Source, error) {
	if !s.mu.TryLock() {
		return Source{}, ErrBusy
	}
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 {
		return Source{}, ErrNotFound
	}
	source := s.sources[i]
	if !ValidInterval(interval) || (source.URL == "" && interval != 0) {
		return Source{}, fmt.Errorf("%w: schedule must be manual or between 1 hour and 7 days, and only applies to URL sources", ErrInvalid)
	}
	source.UpdateInterval = interval
	return s.save(ctx, source)
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
	s.publish()
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
	previousFailures := source.ConsecutiveFailures
	attempted := s.now().UTC()
	source.LastAttemptAt = &attempted
	domains, err := s.fetch(ctx, source.URL)
	if err != nil {
		// The previous domains stay in place: only bookkeeping changes.
		source.LastError = "Download or list validation failed; previous domains retained."
		source.ConsecutiveFailures++
		if _, saveErr := s.save(ctx, source); saveErr != nil {
			return Source{}, fmt.Errorf("record update failure: %w", saveErr)
		}
		s.notify(source, previousFailures, err)
		return source, fmt.Errorf("source update failed")
	}
	source.Domains = domains
	source.LastUpdatedAt = &attempted
	source.LastError = ""
	source.ConsecutiveFailures = 0
	saved, err := s.save(ctx, source)
	if err == nil {
		s.notify(saved, previousFailures, nil)
	}
	return saved, err
}

func (s *Service) notify(source Source, previousFailures int, err error) {
	if s.onRefresh != nil {
		s.onRefresh(source, previousFailures, err)
	}
}

// RefreshDue refreshes every scheduled source whose next update time has
// passed and returns how many attempts were made. While any other blocklist
// operation holds the lock (a manual refresh can take seconds) the pass is
// skipped; due sources are picked up on the next tick.
func (s *Service) RefreshDue(ctx context.Context) int {
	if !s.mu.TryLock() {
		return 0
	}
	now := s.now()
	var due []int64
	for _, source := range s.sources {
		if next := s.nextUpdate(source); next != nil && !next.After(now) {
			due = append(due, source.ID)
		}
	}
	s.mu.Unlock()
	attempts := 0
	for _, id := range due {
		if ctx.Err() != nil {
			break
		}
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err := s.Refresh(refreshCtx, id)
		cancel()
		if !errors.Is(err, ErrBusy) && !errors.Is(err, ErrNotFound) {
			attempts++
		}
	}
	return attempts
}

// RunScheduler checks for due sources every interval until ctx ends.
func (s *Service) RunScheduler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RefreshDue(ctx)
		}
	}
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
	s.publish()
	return nil
}
