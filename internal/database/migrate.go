package database

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
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
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=?)", version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		migration, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(migration)); err != nil {
			return fmt.Errorf("migration %d: %w", version, err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO schema_migrations(version) VALUES (?)", version); err != nil {
			return err
		}
	}
	return tx.Commit()
}
