package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "test.db")
	for range 2 {
		s, err := Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Ping(context.Background()); err != nil {
			t.Fatal(err)
		}
		var n int
		if err = s.db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n); err != nil || n != 2 {
			t.Fatalf("migration: %d %v", n, err)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
