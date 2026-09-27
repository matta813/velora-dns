package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/querylog"
)

func TestClientsPersistAndActivity(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "velora.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	saved, err := store.SaveClient(ctx, clients.Client{Name: "Living Room TV", Addresses: []string{"192.168.1.200", "2001:db8::200"}, Group: "Media", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveClient(ctx, clients.Client{Name: "living room tv", Addresses: []string{"192.168.1.201"}}); !errors.Is(err, clients.ErrExists) {
		t.Fatalf("case-insensitive unique name: %v", err)
	}
	loaded, err := store.LoadClients(ctx)
	if err != nil || len(loaded) != 1 || loaded[0].ID != saved.ID || len(loaded[0].Addresses) != 2 || loaded[0].Group != "Media" || !loaded[0].Enabled {
		t.Fatalf("load: %v %+v", err, loaded)
	}
	now := time.Now().UTC()
	entries := []querylog.Entry{
		{OccurredAt: now.Add(-2 * time.Hour), ClientIP: "192.168.1.200", Domain: "a.test.", Type: "A", Rcode: "NOERROR", Source: "cache"},
		{OccurredAt: now.Add(-time.Minute), ClientIP: "192.168.1.200", Domain: "b.test.", Type: "A", Rcode: "NOERROR", Source: "cache"},
		{OccurredAt: now.Add(-time.Minute), ClientIP: "192.168.1.9", Domain: "c.test.", Type: "A", Rcode: "NOERROR", Source: "cache"},
		{OccurredAt: now.Add(-48 * time.Hour), ClientIP: "192.168.1.8", Domain: "old.test.", Type: "A", Rcode: "NOERROR", Source: "cache"},
	}
	if err = store.WriteQueries(ctx, entries, now.Add(-72*time.Hour), 1000); err != nil {
		t.Fatal(err)
	}
	activity, err := store.ClientActivity(ctx, now.Add(-24*time.Hour), 100)
	if err != nil || len(activity) != 2 || activity[0].ClientIP != "192.168.1.200" || activity[0].Queries != 2 || activity[0].LastSeen.Before(now.Add(-2*time.Minute)) {
		t.Fatalf("activity: %v %+v", err, activity)
	}
	if _, err = store.ClientActivity(ctx, now, 0); err == nil {
		t.Fatal("limit must be validated")
	}
}
