package database

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStatisticsPersistAcrossReopenAndRejectInvalidRows(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "statistics.db")
	store, err := Open(ctx, "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if initial, err := store.LoadStatistics(ctx); err != nil || !reflect.DeepEqual(initial, Statistics{}) {
		t.Fatalf("initial: %+v %v", initial, err)
	}
	want := Statistics{Queries: 31, Blocked: 4, CacheHits: 8, CacheMisses: 12, RateLimitRejections: 2, UpstreamRequests: map[string]uint64{"127.0.0.1:53": 5}, UpstreamErrors: map[string]uint64{"127.0.0.1:53": 1}}
	if err := store.SaveStatistics(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got, err := store.LoadStatistics(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("restored: %+v %v", got, err)
	}
	if err := store.SaveStatistics(ctx, Statistics{Queries: math.MaxUint64}); err == nil {
		t.Fatal("overflow accepted")
	}
	if err := store.SaveStatistics(ctx, Statistics{Queries: 1, Blocked: 2}); !errors.Is(err, ErrInvalidStatistics) {
		t.Fatalf("blocked > queries: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE statistics SET cache_hits=-1 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadStatistics(ctx); !errors.Is(err, ErrInvalidStatistics) {
		t.Fatalf("corrupt row: %v", err)
	}
	if err := store.SaveStatistics(ctx, Statistics{}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadStatistics(ctx); err != nil || got.Queries != 0 || got.CacheHits != 0 || len(got.UpstreamRequests) != 0 {
		t.Fatalf("reset: %+v %v", got, err)
	}
}
