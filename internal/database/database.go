// Package database owns management persistence; DNS cache never depends on SQL.
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db     *sql.DB
	driver string // "sqlite" or "postgres"
}

// Open opens a database connection using the specified driver.
// driver must be "sqlite" or "postgres".
// For sqlite, path is a file path; for postgres, path is a connection URL.
func Open(ctx context.Context, driver, path string) (*Store, error) {
	switch driver {
	case "sqlite":
		return openSQLite(ctx, path)
	case "postgres":
		return openPostgres(ctx, path)
	default:
		return nil, fmt.Errorf("unsupported database driver: %q", driver)
	}
}

func openSQLite(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(e error) (*Store, error) { _ = db.Close(); return nil, e }
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err = db.ExecContext(ctx, q); err != nil {
			return fail(err)
		}
	}
	s := &Store{db: db, driver: "sqlite"}
	if err = s.migrate(ctx); err != nil {
		return fail(err)
	}
	return s, nil
}

func openPostgres(ctx context.Context, url string) (*Store, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	fail := func(e error) (*Store, error) { _ = db.Close(); return nil, e }
	if err = db.PingContext(ctx); err != nil {
		return fail(fmt.Errorf("ping postgres: %w", err))
	}
	s := &Store{db: db, driver: "postgres"}
	if err = s.migrate(ctx); err != nil {
		return fail(err)
	}
	return s, nil
}

func (s *Store) Driver() string                 { return s.driver }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Close() error {
	if s.driver == "sqlite" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
		closeErr := s.db.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	return s.db.Close()
}

// placeholder returns the appropriate placeholder for the driver.
// SQLite uses "?", PostgreSQL uses "$1", "$2", etc.
func (s *Store) placeholder(n int) string {
	if s.driver == "postgres" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// insertReturning returns the suffix to append to an INSERT statement to get the last insert id.
// SQLite: no suffix (use LastInsertId).
// PostgreSQL: " RETURNING id"
func (s *Store) insertReturning() string {
	if s.driver == "postgres" {
		return " RETURNING id"
	}
	return ""
}
