package database

import (
	"context"
	"github.com/matta813/velora-dns/internal/querylog"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryRoundTripLimitsAndLiteralFilters(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UTC()
	entries := []querylog.Entry{}
	for i := range 6 {
		entries = append(entries, querylog.Entry{OccurredAt: now.Add(time.Duration(i) * time.Microsecond), Domain: "Example.Test.", ClientIP: "192.0.2.1", Type: "A", Rcode: "NOERROR", Source: "cache", Duration: 1200 * time.Microsecond, CacheHit: true})
	}
	if err = db.WriteQueries(ctx, entries, now.Add(-time.Hour), 3); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListQueries(ctx, querylog.Filter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != 6 || got[0].Domain != "example.test." || !got[0].CacheHit || got[0].Duration != 1200*time.Microsecond || !got[0].OccurredAt.Equal(entries[5].OccurredAt) {
		t.Fatalf("round trip: %+v", got)
	}
	for _, f := range []querylog.Filter{{Limit: 100, Domain: "%"}, {Limit: 100, Client: "192.0.2"}, {Limit: 100, Type: "AAAA"}, {Limit: 100, Source: "blocked"}} {
		got, err = db.ListQueries(ctx, f)
		if err != nil || len(got) != 0 {
			t.Fatalf("nonmatching filter returned records: %+v %v", got, err)
		}
	}
	got, err = db.ListQueries(ctx, querylog.Filter{Limit: 1, Before: 6})
	if err != nil || len(got) != 1 || got[0].ID != 5 {
		t.Fatalf("cursor: %+v %v", got, err)
	}
	if err = db.WriteQueries(ctx, nil, now.Add(time.Minute), 3); err != nil {
		t.Fatal(err)
	}
	got, err = db.ListQueries(ctx, querylog.Filter{Limit: 100})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("retention/empty list: %+v %v", got, err)
	}
}
