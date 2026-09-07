package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type BlocklistSource struct {
	ID            int64
	Name, URL     string
	Enabled       bool
	LastUpdatedAt *time.Time
	LastError     string
}

func (s *Store) ListBlocklistSources(ctx context.Context) ([]BlocklistSource, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,url,enabled,last_updated_at,last_error FROM blocklist_sources ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlocklistSource
	for rows.Next() {
		var x BlocklistSource
		var updated sql.NullTime
		if err := rows.Scan(&x.ID, &x.Name, &x.URL, &x.Enabled, &updated, &x.LastError); err != nil {
			return nil, err
		}
		if updated.Valid {
			x.LastUpdatedAt = &updated.Time
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) CreateBlocklistSource(ctx context.Context, name, rawURL string, enabled bool) (BlocklistSource, error) {
	r, err := s.db.ExecContext(ctx, "INSERT INTO blocklist_sources(name,url,enabled) VALUES(?,?,?)", name, rawURL, enabled)
	if err != nil {
		return BlocklistSource{}, err
	}
	id, err := r.LastInsertId()
	return BlocklistSource{ID: id, Name: name, URL: rawURL, Enabled: enabled}, err
}

// ReplaceBlocklistDomains atomically publishes a successful source refresh. A failed
// refresh must call RecordBlocklistError instead, preserving prior domains.
func (s *Store) ReplaceBlocklistDomains(ctx context.Context, id int64, domains []string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM blocklist_domains WHERE source_id=?", id); err != nil {
		return err
	}
	for _, domain := range domains {
		if _, err = tx.ExecContext(ctx, "INSERT INTO blocklist_domains(source_id,domain) VALUES(?,?)", id, domain); err != nil {
			return err
		}
	}
	r, err := tx.ExecContext(ctx, "UPDATE blocklist_sources SET last_updated_at=?,last_error='' WHERE id=?", now.UTC(), id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("blocklist source not found")
	}
	return tx.Commit()
}
func (s *Store) RecordBlocklistError(ctx context.Context, id int64, message string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE blocklist_sources SET last_error=? WHERE id=?", message, id)
	return err
}
func (s *Store) LoadBlocklistDomains(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT d.domain FROM blocklist_domains d JOIN blocklist_sources s ON s.id=d.source_id WHERE s.enabled=1 ORDER BY d.domain")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var domain string
		if err = rows.Scan(&domain); err != nil {
			return nil, err
		}
		out = append(out, domain)
	}
	return out, rows.Err()
}
func (s *Store) GetBlocklistSource(ctx context.Context, id int64) (BlocklistSource, error) {
	var source BlocklistSource
	var updated sql.NullTime
	err := s.db.QueryRowContext(ctx, "SELECT id,name,url,enabled,last_updated_at,last_error FROM blocklist_sources WHERE id=?", id).Scan(&source.ID, &source.Name, &source.URL, &source.Enabled, &updated, &source.LastError)
	if err != nil {
		return source, err
	}
	if updated.Valid {
		source.LastUpdatedAt = &updated.Time
	}
	return source, nil
}
