package querylog

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testStore struct {
	entries []Entry
	fail    bool
}

func (s *testStore) WriteQueries(_ context.Context, entries []Entry, _ time.Time, _ int) error {
	if s.fail {
		return errors.New("storage offline")
	}
	s.entries = append(s.entries, entries...)
	return nil
}
func TestQueueBoundedAndShutdownDrains(t *testing.T) {
	store := &testStore{}
	log := New(store, true, 2, time.Hour, 100)
	for range 3 {
		log.Record(Entry{Domain: "example.test"})
	}
	if log.Snapshot().Dropped != 1 {
		t.Fatal("queue overflow not counted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	log.Run(ctx)
	if len(store.entries) != 2 || log.Snapshot().Written != 2 {
		t.Fatalf("accepted entries not drained: %+v", log.Snapshot())
	}
	log.Record(Entry{})
	if log.Snapshot().Dropped != 2 {
		t.Fatal("accepted after shutdown")
	}
}
func TestDisabledAndStorageFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &testStore{}
	log := New(store, false, 1, time.Hour, 100)
	log.Record(Entry{})
	log.Run(ctx)
	if len(store.entries) != 0 {
		t.Fatal("disabled logging retained query")
	}
	store.fail = true
	log = New(store, true, 1, time.Hour, 100)
	log.Record(Entry{})
	log.Run(ctx)
	if log.Snapshot().Dropped != 1 || log.Snapshot().Errors == 0 {
		t.Fatal("storage loss invisible")
	}
}
