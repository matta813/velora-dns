package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/matta813/velora-dns/internal/filtering"
	"modernc.org/sqlite"
	"time"
)

var _ filtering.SourceStore = (*Store)(nil)

func sourceError(err error) error {
	var e *sqlite.Error
	if errors.As(err, &e) && e.Code()&255 == 19 {
		return filtering.ErrExists
	}
	return err
}
func (s *Store) LoadSources(ctx context.Context) ([]filtering.Source, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, "SELECT id,name,url,enabled,last_updated_at,last_error FROM blocklist_sources ORDER BY id LIMIT ?", filtering.MaxSources+1)
	if err != nil {
		return nil, err
	}
	out := []filtering.Source{}
	index := map[int64]int{}
	for rows.Next() {
		var source filtering.Source
		var updated sql.NullString
		if err = rows.Scan(&source.ID, &source.Name, &source.URL, &source.Enabled, &updated, &source.LastError); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if updated.Valid {
			var timestamp time.Time
			// Accept timestamps written by the initial source implementation as well.
			for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999 -0700 MST"} {
				timestamp, err = time.Parse(layout, updated.String)
				if err == nil {
					break
				}
			}
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("invalid source timestamp: %w", err)
			}
			source.LastUpdatedAt = &timestamp
		}
		index[source.ID] = len(out)
		out = append(out, source)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	if len(out) > filtering.MaxSources {
		return nil, fmt.Errorf("stored source limit exceeded")
	}
	rows, err = tx.QueryContext(ctx, "SELECT source_id,domain FROM blocklist_domains ORDER BY source_id,domain LIMIT ?", filtering.MaxTotalDomains+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var id int64
		var domain string
		if err = rows.Scan(&id, &domain); err != nil {
			return nil, err
		}
		i, ok := index[id]
		if !ok {
			return nil, fmt.Errorf("missing source")
		}
		out[i].Domains = append(out[i].Domains, domain)
		count++
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	if count > filtering.MaxTotalDomains {
		return nil, fmt.Errorf("stored domain limit exceeded")
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Store) SaveSource(ctx context.Context, source filtering.Source) (filtering.Source, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return source, err
	}
	defer func() { _ = tx.Rollback() }()
	var timestamp any
	if source.LastUpdatedAt != nil {
		timestamp = source.LastUpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	if source.ID == 0 {
		result, e := tx.ExecContext(ctx, "INSERT INTO blocklist_sources(name,url,enabled,last_updated_at,last_error) VALUES(?,?,?,?,?)", source.Name, source.URL, source.Enabled, timestamp, source.LastError)
		if e != nil {
			return source, sourceError(e)
		}
		source.ID, err = result.LastInsertId()
		if err != nil {
			return source, err
		}
	} else {
		result, e := tx.ExecContext(ctx, "UPDATE blocklist_sources SET name=?,url=?,enabled=?,last_updated_at=?,last_error=? WHERE id=?", source.Name, source.URL, source.Enabled, timestamp, source.LastError, source.ID)
		if e != nil {
			return source, sourceError(e)
		}
		n, e := result.RowsAffected()
		if e != nil {
			return source, e
		}
		if n != 1 {
			return source, filtering.ErrNotFound
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM blocklist_domains WHERE source_id=?", source.ID); err != nil {
		return source, err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO blocklist_domains(source_id,domain) VALUES(?,?)")
	if err != nil {
		return source, err
	}
	defer func() { _ = stmt.Close() }()
	for _, domain := range source.Domains {
		if _, err = stmt.ExecContext(ctx, source.ID, domain); err != nil {
			return source, err
		}
	}
	if err = tx.Commit(); err != nil {
		return source, err
	}
	return source, nil
}
func (s *Store) DeleteSource(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM blocklist_sources WHERE id=?", id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return filtering.ErrNotFound
	}
	return nil
}
