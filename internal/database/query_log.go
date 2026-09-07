package database

import (
	"context"
	"time"

	"github.com/matta813/velora-dns/internal/querylog"
)

func (s *Store) InsertQuery(ctx context.Context, entry querylog.Entry) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO query_log(occurred_at,client_ip,domain,query_type,response_code,duration_micros,source,upstream,cache_hit) VALUES(?,?,?,?,?,?,?,?,?)", entry.OccurredAt.UTC(), entry.ClientIP, entry.Domain, entry.Type, entry.Rcode, entry.Duration.Microseconds(), entry.Source, entry.Upstream, entry.CacheHit)
	return err
}
func (s *Store) PurgeQueries(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM query_log WHERE occurred_at < ?", before.UTC())
	return err
}
