package database

import (
	"context"
	"fmt"
	"time"

	"github.com/matta813/velora-dns/internal/replication"
)

func (s *Store) SaveConfigVersion(ctx context.Context, v replication.ConfigVersion) error {
	p := s.placeholder
	insertSQL := fmt.Sprintf("INSERT INTO config_versions(version,config_hash,applied_by,applied_at) VALUES(%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), s.insertReturning())
	if s.driver == "postgres" {
		_, err := s.db.ExecContext(ctx, insertSQL, v.Version, v.ConfigHash, v.AppliedBy, v.AppliedAt.UTC().Format(time.RFC3339))
		return err
	}
	_, err := s.db.ExecContext(ctx, insertSQL, v.Version, v.ConfigHash, v.AppliedBy, v.AppliedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetLatestVersion(ctx context.Context) (replication.ConfigVersion, error) {
	var v replication.ConfigVersion
	err := s.db.QueryRowContext(ctx, "SELECT version,config_hash,applied_by,applied_at FROM config_versions ORDER BY version DESC LIMIT 1").Scan(&v.Version, &v.ConfigHash, &v.AppliedBy, &v.AppliedAt)
	return v, err
}

func (s *Store) ListVersions(ctx context.Context, limit int) ([]replication.ConfigVersion, error) {
	p := s.placeholder
	query := fmt.Sprintf("SELECT version,config_hash,applied_by,applied_at FROM config_versions ORDER BY version DESC LIMIT %s", p(1))
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var versions []replication.ConfigVersion
	for rows.Next() {
		var v replication.ConfigVersion
		if err = rows.Scan(&v.Version, &v.ConfigHash, &v.AppliedBy, &v.AppliedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}
