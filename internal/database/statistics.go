package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

type Statistics struct {
	Queries             uint64
	Blocked             uint64
	CacheHits           uint64
	CacheMisses         uint64
	RateLimitRejections uint64
	UpstreamRequests    map[string]uint64
	UpstreamErrors      map[string]uint64
}

var ErrInvalidStatistics = errors.New("invalid persisted statistics")

func (s *Store) LoadStatistics(ctx context.Context) (Statistics, error) {
	var version int
	var queries, blocked, hits, misses, rejected int64
	var requestsJSON, errorsJSON string
	err := s.db.QueryRowContext(ctx, "SELECT format_version,queries,blocked,cache_hits,cache_misses,rate_limit_rejections,upstream_requests,upstream_errors FROM statistics WHERE id=1").Scan(&version, &queries, &blocked, &hits, &misses, &rejected, &requestsJSON, &errorsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return Statistics{}, nil
	}
	if err != nil {
		return Statistics{}, err
	}
	if version != 1 || queries < 0 || blocked < 0 || blocked > queries || hits < 0 || misses < 0 || rejected < 0 {
		return Statistics{}, ErrInvalidStatistics
	}
	stats := Statistics{Queries: uint64(queries), Blocked: uint64(blocked), CacheHits: uint64(hits), CacheMisses: uint64(misses), RateLimitRejections: uint64(rejected)}
	if json.Unmarshal([]byte(requestsJSON), &stats.UpstreamRequests) != nil || json.Unmarshal([]byte(errorsJSON), &stats.UpstreamErrors) != nil || stats.UpstreamRequests == nil || stats.UpstreamErrors == nil {
		return Statistics{}, ErrInvalidStatistics
	}
	for server, count := range stats.UpstreamRequests {
		if server == "" || count > math.MaxInt64 {
			return Statistics{}, ErrInvalidStatistics
		}
	}
	for server, count := range stats.UpstreamErrors {
		if server == "" || count > math.MaxInt64 || count > stats.UpstreamRequests[server] {
			return Statistics{}, ErrInvalidStatistics
		}
	}
	return stats, nil
}

func (s *Store) SaveStatistics(ctx context.Context, stats Statistics) error {
	for _, value := range []uint64{stats.Queries, stats.Blocked, stats.CacheHits, stats.CacheMisses, stats.RateLimitRejections} {
		if value > math.MaxInt64 {
			return fmt.Errorf("statistics counter exceeds storage range")
		}
	}
	if stats.Blocked > stats.Queries {
		return ErrInvalidStatistics
	}
	for server, count := range stats.UpstreamRequests {
		if server == "" || count > math.MaxInt64 {
			return ErrInvalidStatistics
		}
	}
	for server, count := range stats.UpstreamErrors {
		if server == "" || count > math.MaxInt64 || count > stats.UpstreamRequests[server] {
			return ErrInvalidStatistics
		}
	}
	requestsJSON, err := json.Marshal(stats.UpstreamRequests)
	if err != nil {
		return err
	}
	errorsJSON, err := json.Marshal(stats.UpstreamErrors)
	if err != nil {
		return err
	}
	if stats.UpstreamRequests == nil {
		requestsJSON = []byte("{}")
	}
	if stats.UpstreamErrors == nil {
		errorsJSON = []byte("{}")
	}
	p := s.placeholder
	query := fmt.Sprintf("INSERT INTO statistics(id,format_version,queries,blocked,cache_hits,cache_misses,rate_limit_rejections,upstream_requests,upstream_errors,saved_at) VALUES(1,1,%s,%s,%s,%s,%s,%s,%s,CURRENT_TIMESTAMP) ON CONFLICT(id) DO UPDATE SET queries=excluded.queries,blocked=excluded.blocked,cache_hits=excluded.cache_hits,cache_misses=excluded.cache_misses,rate_limit_rejections=excluded.rate_limit_rejections,upstream_requests=excluded.upstream_requests,upstream_errors=excluded.upstream_errors,saved_at=CURRENT_TIMESTAMP", p(1), p(2), p(3), p(4), p(5), p(6), p(7))
	_, err = s.db.ExecContext(ctx, query, int64(stats.Queries), int64(stats.Blocked), int64(stats.CacheHits), int64(stats.CacheMisses), int64(stats.RateLimitRejections), string(requestsJSON), string(errorsJSON))
	return err
}
