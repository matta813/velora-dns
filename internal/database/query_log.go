package database

import (
	"context"
	"fmt"
	"github.com/matta813/velora-dns/internal/querylog"
	"strings"
	"time"
)

const queryTimeFormat = "2006-01-02T15:04:05.000000000Z"

// WriteQueries bounds both age and row count in the same transaction as insertion.
func (s *Store) WriteQueries(ctx context.Context, entries []querylog.Entry, before time.Time, maxRows int) error {
	if maxRows < 1 {
		return fmt.Errorf("invalid query history capacity")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, entry := range entries {
		if _, err = tx.ExecContext(ctx, "INSERT INTO query_log(occurred_at,client_ip,domain,query_type,response_code,duration_micros,source,upstream,cache_hit) VALUES(?,?,?,?,?,?,?,?,?)", entry.OccurredAt.UTC().Format(queryTimeFormat), entry.ClientIP, strings.ToLower(entry.Domain), entry.Type, entry.Rcode, entry.Duration.Microseconds(), entry.Source, entry.Upstream, entry.CacheHit); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM query_log WHERE occurred_at < ?", before.UTC().Format(queryTimeFormat)); err != nil {
		return err
	}
	// Monotonic IDs bound storage without scanning retained history on every write.
	if _, err = tx.ExecContext(ctx, "DELETE FROM query_log WHERE id <= (SELECT COALESCE(MAX(id),0)-? FROM query_log)", maxRows); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ListQueries(ctx context.Context, filter querylog.Filter) ([]querylog.Entry, error) {
	if filter.Limit < 1 || filter.Limit > 500 {
		return nil, fmt.Errorf("invalid query limit")
	}
	statement := "SELECT id,occurred_at,client_ip,domain,query_type,response_code,duration_micros,source,upstream,cache_hit FROM query_log WHERE 1=1"
	args := []any{}
	if filter.Domain != "" {
		statement += " AND instr(domain,?)>0"
		args = append(args, strings.ToLower(filter.Domain))
	}
	for _, p := range []struct{ column, value string }{{"client_ip", filter.Client}, {"query_type", filter.Type}, {"source", filter.Source}} {
		if p.value != "" {
			statement += " AND " + p.column + "=?"
			args = append(args, p.value)
		}
	}
	if filter.Before > 0 {
		statement += " AND id<?"
		args = append(args, filter.Before)
	}
	statement += " ORDER BY id DESC LIMIT ?"
	args = append(args, filter.Limit)
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []querylog.Entry{}
	for rows.Next() {
		var e querylog.Entry
		var timestamp string
		var micros int64
		if err = rows.Scan(&e.ID, &timestamp, &e.ClientIP, &e.Domain, &e.Type, &e.Rcode, &micros, &e.Source, &e.Upstream, &e.CacheHit); err != nil {
			return nil, err
		}
		e.OccurredAt, err = time.Parse(queryTimeFormat, timestamp)
		if err != nil {
			return nil, fmt.Errorf("invalid stored query timestamp: %w", err)
		}
		e.Duration = time.Duration(micros) * time.Microsecond
		out = append(out, e)
	}
	return out, rows.Err()
}
