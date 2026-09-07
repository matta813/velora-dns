package database

import (
	"context"
	"time"

	"github.com/matta813/velora-dns/internal/querylog"
)

type QueryFilter struct {
	Domain, Client, Type, Source string
	Limit                        int
}

func (s *Store) InsertQuery(ctx context.Context, entry querylog.Entry) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO query_log(occurred_at,client_ip,domain,query_type,response_code,duration_micros,source,upstream,cache_hit) VALUES(?,?,?,?,?,?,?,?,?)", entry.OccurredAt.UTC(), entry.ClientIP, entry.Domain, entry.Type, entry.Rcode, entry.Duration.Microseconds(), entry.Source, entry.Upstream, entry.CacheHit)
	return err
}
func (s *Store) PurgeQueries(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM query_log WHERE occurred_at < ?", before.UTC())
	return err
}
func (s *Store) ListQueries(ctx context.Context, filter QueryFilter) ([]querylog.Entry, error) {
	if filter.Limit < 1 || filter.Limit > 500 {
		filter.Limit = 100
	}
	rows, err := s.db.QueryContext(ctx, "SELECT occurred_at,client_ip,domain,query_type,response_code,duration_micros,source,upstream,cache_hit FROM query_log WHERE domain LIKE ? AND client_ip LIKE ? AND query_type LIKE ? AND source LIKE ? ORDER BY occurred_at DESC LIMIT ?", "%"+filter.Domain+"%", "%"+filter.Client+"%", "%"+filter.Type+"%", "%"+filter.Source+"%", filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []querylog.Entry
	for rows.Next() {
		var entry querylog.Entry
		var micros int64
		if err = rows.Scan(&entry.OccurredAt, &entry.ClientIP, &entry.Domain, &entry.Type, &entry.Rcode, &micros, &entry.Source, &entry.Upstream, &entry.CacheHit); err != nil {
			return nil, err
		}
		entry.Duration = time.Duration(micros) * time.Microsecond
		out = append(out, entry)
	}
	return out, rows.Err()
}
