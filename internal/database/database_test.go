package database

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresMigrationAdaptationUsesBinaryType(t *testing.T) {
	s := &Store{driver: "postgres"}
	adapted := s.adaptMigration("token_hash BLOB PRIMARY KEY, csrf_token BLOB NOT NULL")
	if strings.Contains(adapted, "BLOB") || !strings.Contains(adapted, "token_hash BYTEA PRIMARY KEY") || !strings.Contains(adapted, "csrf_token BYTEA NOT NULL") {
		t.Fatalf("unexpected PostgreSQL migration: %s", adapted)
	}
}

func TestPostgresMigrationAdaptationUsesBooleanTypes(t *testing.T) {
	s := &Store{driver: "postgres"}
	adapted := s.adaptMigration("enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)), disabled INTEGER NOT NULL DEFAULT 0 CHECK(disabled IN (0, 1)), cache_hit INTEGER NOT NULL DEFAULT 0")
	if !strings.Contains(adapted, "enabled BOOLEAN NOT NULL DEFAULT true") || !strings.Contains(adapted, "CHECK(enabled IN (false, true))") {
		t.Fatalf("enabled bool migration not adapted: %s", adapted)
	}
	if !strings.Contains(adapted, "disabled BOOLEAN NOT NULL DEFAULT false") || !strings.Contains(adapted, "CHECK(disabled IN (false, true))") {
		t.Fatalf("disabled bool migration not adapted: %s", adapted)
	}
	if !strings.Contains(adapted, "cache_hit BOOLEAN NOT NULL DEFAULT false") {
		t.Fatalf("cache_hit bool migration not adapted: %s", adapted)
	}
}

func TestOpenReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "test.db")
	for range 2 {
		s, err := Open(context.Background(), "sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Ping(context.Background()); err != nil {
			t.Fatal(err)
		}
		var n int
		if err = s.db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n); err != nil || n != 11 {
			t.Fatalf("migration: %d %v", n, err)
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
