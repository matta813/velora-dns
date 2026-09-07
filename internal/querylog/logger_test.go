package querylog

import (
	"context"
	"sync"
	"testing"
	"time"
)

type testStore struct {
	mu      sync.Mutex
	entries []Entry
}

func (s *testStore) InsertQuery(_ context.Context, entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	return nil
}
func (*testStore) PurgeQueries(context.Context, time.Time) error { return nil }
func TestDisabledLoggerDoesNotWrite(t *testing.T) {
	store := &testStore{}
	logger := New(store, false, 1, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger.Run(ctx)
	logger.Record(Entry{Domain: "example.test"})
	time.Sleep(10 * time.Millisecond)
	if len(store.entries) != 0 {
		t.Fatal("disabled logger wrote entry")
	}
}
func TestLoggerWritesAsynchronously(t *testing.T) {
	store := &testStore{}
	logger := New(store, true, 1, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger.Run(ctx)
	logger.Record(Entry{Domain: "example.test"})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := len(store.entries)
		store.mu.Unlock()
		if n == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("entry was not written")
}
