package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	createSQL := "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"
	if _, err = tx.ExecContext(ctx, createSQL); err != nil {
		return err
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, file := range files {
		prefix, _, ok := strings.Cut(file.Name(), "_")
		if !ok {
			return fmt.Errorf("invalid migration filename")
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return err
		}
		var exists bool
		checkSQL := "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=" + s.placeholder(1) + ")"
		if err = tx.QueryRowContext(ctx, checkSQL, version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		migration, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			return err
		}
		sql := s.adaptMigration(string(migration))
		if _, err = tx.ExecContext(ctx, sql); err != nil {
			return fmt.Errorf("migration %d: %w", version, err)
		}
		insertSQL := "INSERT INTO schema_migrations(version) VALUES(" + s.placeholder(1) + ") ON CONFLICT DO NOTHING"
		if _, err = tx.ExecContext(ctx, insertSQL, version); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// adaptMigration converts SQLite-specific SQL to PostgreSQL where needed.
// This is a targeted replacement for common SQLite idioms.
func (s *Store) adaptMigration(sql string) string {
	if s.driver != "postgres" {
		return sql
	}
	// Replace AUTOINCREMENT with PostgreSQL SERIAL (handled by column type)
	sql = strings.ReplaceAll(sql, "INTEGER PRIMARY KEY AUTOINCREMENT", "SERIAL PRIMARY KEY")
	// Replace INSERT OR IGNORE with ON CONFLICT DO NOTHING
	sql = strings.ReplaceAll(sql, "INSERT OR IGNORE", "INSERT")
	// Replace COLLATE NOCASE with case-insensitive comparison (PostgreSQL uses citext or LOWER)
	sql = strings.ReplaceAll(sql, " COLLATE NOCASE", "")
	// PostgreSQL uses BYTEA for arbitrary binary values such as token hashes.
	sql = strings.ReplaceAll(sql, "BLOB", "BYTEA")
	// Rewrite SQLite integer booleans to PostgreSQL BOOLEAN columns.
	sql = strings.ReplaceAll(sql, "enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1))", "enabled BOOLEAN NOT NULL DEFAULT true CHECK(enabled IN (false, true))")
	sql = strings.ReplaceAll(sql, "disabled INTEGER NOT NULL DEFAULT 0 CHECK(disabled IN (0, 1))", "disabled BOOLEAN NOT NULL DEFAULT false CHECK(disabled IN (false, true))")
	sql = strings.ReplaceAll(sql, "cache_hit INTEGER NOT NULL DEFAULT 0", "cache_hit BOOLEAN NOT NULL DEFAULT false")
	return sql
}
