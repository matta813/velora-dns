package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/filtering"
)

func TestBlocklistScheduleSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "velora.db")
	store, err := Open(ctx, "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	attempted := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	saved, err := store.SaveSource(ctx, filtering.Source{Name: "hosts", URL: "https://example.org/hosts", Enabled: true, UpdateInterval: 6 * 3600, LastAttemptAt: &attempted, ConsecutiveFailures: 2, LastError: "failed", Domains: []string{"ads.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err = Open(ctx, "sqlite", path); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	sources, err := store.LoadSources(ctx)
	if err != nil || len(sources) != 1 {
		t.Fatalf("load: %v %+v", err, sources)
	}
	got := sources[0]
	if got.ID != saved.ID || got.UpdateInterval != 6*3600 || got.ConsecutiveFailures != 2 || got.LastAttemptAt == nil || !got.LastAttemptAt.Equal(attempted) || got.LastUpdatedAt != nil || len(got.Domains) != 1 {
		t.Fatalf("schedule not persisted: %+v", got)
	}
}
