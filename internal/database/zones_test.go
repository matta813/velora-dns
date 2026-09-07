package database

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/matta813/velora-dns/internal/zones"
	wire "github.com/miekg/dns"
)

func TestZonePersistenceAndRevisions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "zones.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	var flushes atomic.Int32
	service, err := zones.New(ctx, db, func() { flushes.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	z, err := service.Create(ctx, zones.Zone{Name: "Home.Test", Records: []zones.Record{{Name: "host", Type: "A", TTL: 60, Value: "192.0.2.1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if z.ID == 0 || z.Records[0].ID == 0 || z.Name != "home.test." || z.Revision != 1 {
		t.Fatalf("bad persisted zone: %+v", z)
	}
	id := z.Records[0].ID
	z.Records[0].Value = "192.0.2.2"
	z, err = service.Update(ctx, z.ID, 1, z)
	if err != nil {
		t.Fatal(err)
	}
	if z.Records[0].ID != id || z.Revision != 2 {
		t.Fatal("IDs not stable")
	}
	if _, err = service.Update(ctx, z.ID, 1, z); !errors.Is(err, zones.ErrConflict) {
		t.Fatal("stale write accepted")
	}
	bad := z
	bad.Records = append([]zones.Record{}, z.Records...)
	bad.Records[0].Value = "invalid"
	if _, err = service.Update(ctx, z.ID, z.Revision, bad); !errors.Is(err, zones.ErrInvalid) {
		t.Fatal("invalid update accepted")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	service, err = zones.New(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := service.Get(z.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Records[0].Value != "192.0.2.2" || restored.Revision != 2 {
		t.Fatalf("lost persisted state: %+v", restored)
	}
	if err = service.Delete(ctx, z.ID, 2); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.db.QueryRow("SELECT count(*) FROM zone_records").Scan(&count); err != nil || count != 0 {
		t.Fatal("records not cascaded")
	}
	if flushes.Load() != 2 {
		t.Fatal("failed mutations invalidated cache")
	}
}
func TestConcurrentZoneUpdateAndDNSReads(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "zones.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	service, err := zones.New(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	z, err := service.Create(ctx, zones.Zone{Name: "home.test"})
	if err != nil {
		t.Fatal(err)
	}
	var success atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 30 {
				q := new(wire.Msg)
				q.SetQuestion("home.test.", wire.TypeSOA)
				m, ok := service.Lookup(q)
				if !ok || len(m.Answer) != 1 {
					t.Error("inconsistent snapshot")
				}
			}
			if _, err := service.Update(ctx, z.ID, z.Revision, z); err == nil {
				success.Add(1)
			} else if !errors.Is(err, zones.ErrConflict) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatalf("successful writers: %d", success.Load())
	}
}
func TestStorageFailureDoesNotPublish(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "zones.db"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := zones.New(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Create(ctx, zones.Zone{Name: "home.test"}); err == nil {
		t.Fatal("closed storage accepted write")
	}
	if len(service.List()) != 0 {
		t.Fatal("failed write published")
	}
}
func TestUpgradeFromFoundation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.db.Exec("DROP TABLE zone_records; DROP TABLE zones; DELETE FROM schema_migrations WHERE version=2"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = zones.New(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
}
