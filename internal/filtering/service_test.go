package filtering

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type testStore struct {
	sources []Source
	id      int64
	err     error
}

func (s *testStore) LoadSources(_ context.Context) ([]Source, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.sources, s.err
}
func (s *testStore) SaveSource(_ context.Context, source Source) (Source, error) {
	if s.err != nil {
		return Source{}, s.err
	}
	if source.ID == 0 {
		s.id++
		source.ID = s.id
		s.sources = append(s.sources, source)
	} else {
		for i := range s.sources {
			if s.sources[i].ID == source.ID {
				s.sources[i] = source
				break
			}
		}
	}
	return source, nil
}
func (s *testStore) DeleteSource(_ context.Context, id int64) error {
	if s.err != nil {
		return s.err
	}
	for i := range s.sources {
		if s.sources[i].ID == id {
			s.sources = append(s.sources[:i], s.sources[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func TestServiceCreateAndBlock(t *testing.T) {
	store := &testStore{}
	service, err := NewService(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.Create(context.Background(), "test", "", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReplaceLocal(context.Background(), source.ID, "0.0.0.0 ads.example\ntracker.example")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		domain  string
		blocked bool
	}{{"ads.example", true}, {"a.ads.example", true}, {"tracker.example", true}, {"safe.example", false}} {
		if service.Blocked(tc.domain) != tc.blocked {
			t.Errorf("%s: blocked=%v", tc.domain, tc.blocked)
		}
	}
}

func TestServiceSetEnabled(t *testing.T) {
	store := &testStore{}
	service, err := NewService(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.Create(context.Background(), "test", "", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReplaceLocal(context.Background(), source.ID, "0.0.0.0 ads.example")
	if err != nil {
		t.Fatal(err)
	}
	if !service.Blocked("ads.example") {
		t.Fatal("expected blocked")
	}
	_, err = service.SetEnabled(context.Background(), source.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if service.Blocked("ads.example") {
		t.Fatal("expected not blocked after disable")
	}
}

func TestServiceDelete(t *testing.T) {
	store := &testStore{}
	service, err := NewService(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := service.Create(context.Background(), "test", "", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReplaceLocal(context.Background(), source.ID, "0.0.0.0 ads.example")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Delete(context.Background(), source.ID); err != nil {
		t.Fatal(err)
	}
	if service.Blocked("ads.example") {
		t.Fatal("expected not blocked after delete")
	}
}

func TestServiceDuplicateName(t *testing.T) {
	store := &testStore{}
	service, err := NewService(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(context.Background(), "test", "", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(context.Background(), "test", "", true, 0)
	if err != ErrExists {
		t.Fatalf("expected ErrExists, got %v", err)
	}
}

type scheduleClock struct{ now time.Time }

func (c *scheduleClock) Now() time.Time { return c.now }

func scheduledService(t *testing.T, fetch func(context.Context, string) ([]string, error)) (*Service, *scheduleClock, Source) {
	t.Helper()
	service, err := NewService(context.Background(), &testStore{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	clock := &scheduleClock{now: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)}
	service.now = clock.Now
	service.fetch = fetch
	source, err := service.Create(context.Background(), "hosts", "https://example.org/hosts", true, 6*3600)
	if err != nil {
		t.Fatal(err)
	}
	return service, clock, source
}

func sourceByID(t *testing.T, service *Service, id int64) Source {
	t.Helper()
	sources, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source.ID == id {
			return source
		}
	}
	t.Fatalf("source %d missing", id)
	return Source{}
}

func TestScheduleValidation(t *testing.T) {
	service, err := NewService(context.Background(), &testStore{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		url      string
		interval int
	}{{"https://example.org/a", 60}, {"https://example.org/b", 8 * 24 * 3600}, {"", 3600}} {
		if _, err := service.Create(context.Background(), tc.url+"x", tc.url, true, tc.interval); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q every %ds: expected ErrInvalid, got %v", tc.url, tc.interval, err)
		}
	}
	local, err := service.Create(context.Background(), "local", "", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetSchedule(context.Background(), local.ID, 3600); !errors.Is(err, ErrInvalid) {
		t.Fatalf("local sources cannot be scheduled: %v", err)
	}
}

func TestScheduledRefreshRunsWhenDueAndKeepsListOnFailure(t *testing.T) {
	fail := false
	var observed []string
	service, clock, source := scheduledService(t, func(context.Context, string) ([]string, error) {
		if fail {
			return nil, errors.New("HTTP 503")
		}
		return []string{"ads.example"}, nil
	})
	service.SetRefreshObserver(func(source Source, previous int, err error) {
		observed = append(observed, fmt.Sprintf("%d->%d:%v", previous, source.ConsecutiveFailures, err != nil))
	})
	if next := sourceByID(t, service, source.ID).NextUpdateAt; next == nil || !next.Equal(clock.now) {
		t.Fatalf("never-attempted source should be due now, got %v", next)
	}
	if n := service.RefreshDue(context.Background()); n != 1 || !service.Blocked("ads.example") {
		t.Fatalf("first refresh: attempts=%d", n)
	}
	if n := service.RefreshDue(context.Background()); n != 0 {
		t.Fatalf("refreshed again before interval: %d", n)
	}
	clock.now = clock.now.Add(6 * time.Hour)
	fail = true
	if n := service.RefreshDue(context.Background()); n != 1 {
		t.Fatalf("due refresh skipped: %d", n)
	}
	got := sourceByID(t, service, source.ID)
	if !service.Blocked("ads.example") || got.ConsecutiveFailures != 1 || got.LastError == "" {
		t.Fatalf("failed refresh must keep previous list: %+v", got)
	}
	if want := clock.now.Add(retryBase); got.NextUpdateAt == nil || !got.NextUpdateAt.Equal(want) {
		t.Fatalf("first retry at %v, want %v", got.NextUpdateAt, want)
	}
	clock.now = clock.now.Add(retryBase)
	service.RefreshDue(context.Background())
	if got = sourceByID(t, service, source.ID); !got.NextUpdateAt.Equal(clock.now.Add(2 * retryBase)) {
		t.Fatalf("backoff did not double: %v", got.NextUpdateAt)
	}
	fail = false
	clock.now = clock.now.Add(2 * retryBase)
	service.RefreshDue(context.Background())
	if got = sourceByID(t, service, source.ID); got.ConsecutiveFailures != 0 || got.LastError != "" {
		t.Fatalf("recovery not recorded: %+v", got)
	}
	want := []string{"0->0:false", "0->1:true", "1->2:true", "2->0:false"}
	if fmt.Sprint(observed) != fmt.Sprint(want) {
		t.Fatalf("observer calls %v, want %v", observed, want)
	}
}

func TestBackoffNeverExceedsInterval(t *testing.T) {
	service, clock, source := scheduledService(t, func(context.Context, string) ([]string, error) { return nil, errors.New("down") })
	if _, err := service.SetSchedule(context.Background(), source.ID, 3600); err != nil {
		t.Fatal(err)
	}
	for range 8 {
		service.RefreshDue(context.Background())
		clock.now = clock.now.Add(2 * time.Hour)
	}
	got := sourceByID(t, service, source.ID)
	if got.LastAttemptAt == nil || got.NextUpdateAt.Sub(*got.LastAttemptAt) != time.Hour {
		t.Fatalf("backoff exceeded interval: attempt=%v next=%v", got.LastAttemptAt, got.NextUpdateAt)
	}
}

func TestScheduledRefreshSkipsBusySourceAndDisabledOrManual(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	service, _, source := scheduledService(t, func(context.Context, string) ([]string, error) {
		started <- struct{}{}
		<-release
		return []string{"ads.example"}, nil
	})
	done := make(chan error, 1)
	go func() { _, err := service.Refresh(context.Background(), source.ID); done <- err }()
	<-started
	if n := service.RefreshDue(context.Background()); n != 0 {
		t.Fatalf("overlapping refresh started: %d", n)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetEnabled(context.Background(), source.ID, false); err != nil {
		t.Fatal(err)
	}
	if next := sourceByID(t, service, source.ID).NextUpdateAt; next != nil {
		t.Fatalf("disabled source scheduled at %v", next)
	}
}
