package database

import (
	"context"
	"errors"
	"fmt"
)

var ErrBackupRequiresSQLite = errors.New("online backup requires SQLite")

// SnapshotSQLite writes a consistent live SQLite copy to a new destination file.
func (s *Store) SnapshotSQLite(ctx context.Context, destination string) (int, error) {
	if s.driver != "sqlite" {
		return 0, ErrBackupRequiresSQLite
	}
	var schema int
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&schema); err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		return 0, fmt.Errorf("snapshot SQLite: %w", err)
	}
	return schema, nil
}

func LatestSchemaVersion() (int, error) {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}
