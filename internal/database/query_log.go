package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/querylog"
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
	p := s.placeholder
	for _, entry := range entries {
		insertSQL := fmt.Sprintf("INSERT INTO query_log(occurred_at,client_ip,domain,query_type,response_code,duration_micros,source,upstream,cache_hit) VALUES(%s,%s,%s,%s,%s,%s,%s,%s,%s)", p(1), p(2), p(3), p(4), p(5), p(6), p(7), p(8), p(9))
		if _, err = tx.ExecContext(ctx, insertSQL, entry.OccurredAt.UTC().Format(queryTimeFormat), entry.ClientIP, strings.ToLower(entry.Domain), entry.Type, entry.Rcode, entry.Duration.Microseconds(), entry.Source, entry.Upstream, entry.CacheHit); err != nil {
			return err
		}
	}
	deleteTimeSQL := fmt.Sprintf("DELETE FROM query_log WHERE occurred_at < %s", p(1))
	if _, err = tx.ExecContext(ctx, deleteTimeSQL, before.UTC().Format(queryTimeFormat)); err != nil {
		return err
	}
	deleteRowsSQL := fmt.Sprintf("DELETE FROM query_log WHERE id <= (SELECT COALESCE(MAX(id),0)-%s FROM query_log)", p(1))
	if _, err = tx.ExecContext(ctx, deleteRowsSQL, maxRows); err != nil {
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
	argIdx := 1
	p := s.placeholder
	if filter.Domain != "" {
		// SQLite uses instr(), PostgreSQL uses position()
		if s.driver == "postgres" {
			statement += " AND position(" + p(argIdx) + " in domain)>0"
		} else {
			statement += " AND instr(domain," + p(argIdx) + ")>0"
		}
		args = append(args, strings.ToLower(filter.Domain))
		argIdx++
	}
	for _, col := range []struct{ column, value string }{{"client_ip", filter.Client}, {"query_type", filter.Type}, {"source", filter.Source}} {
		if col.value != "" {
			statement += " AND " + col.column + "=" + p(argIdx)
			args = append(args, col.value)
			argIdx++
		}
	}
	if filter.Before > 0 {
		statement += " AND id<" + p(argIdx)
		args = append(args, filter.Before)
		argIdx++
	}
	statement += " ORDER BY id DESC LIMIT " + p(argIdx)
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

// QuerySummary computes bounded, on-demand rankings from retained history.
func (s *Store) QuerySummary(ctx context.Context, start, end time.Time, limit int) (querylog.Summary, error) {
	if !start.Before(end) || limit < 1 || limit > 50 {
		return querylog.Summary{}, fmt.Errorf("invalid query summary bounds")
	}
	startText, endText := start.UTC().Format(queryTimeFormat), end.UTC().Format(queryTimeFormat)
	result := querylog.Summary{WindowStart: start.UTC(), WindowEnd: end.UTC(), TopDomains: []querylog.Ranking{}, TopClients: []querylog.Ranking{}}
	p := s.placeholder
	countSQL := fmt.Sprintf("SELECT COUNT(*), COALESCE(SUM(CASE WHEN source='blocked' THEN 1 ELSE 0 END),0) FROM query_log WHERE occurred_at>=%s AND occurred_at<%s", p(1), p(2))
	if err := s.db.QueryRowContext(ctx, countSQL, startText, endText).Scan(&result.Total, &result.Blocked); err != nil {
		return querylog.Summary{}, err
	}
	load := func(column string) ([]querylog.Ranking, error) {
		loadSQL := fmt.Sprintf("SELECT %s, COUNT(*) AS frequency FROM query_log WHERE occurred_at>=%s AND occurred_at<%s GROUP BY %s ORDER BY frequency DESC, %s ASC LIMIT %s", column, p(1), p(2), column, column, p(3))
		rows, err := s.db.QueryContext(ctx, loadSQL, startText, endText, limit)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		out := []querylog.Ranking{}
		for rows.Next() {
			var item querylog.Ranking
			if err = rows.Scan(&item.Value, &item.Count); err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	}
	var err error
	if result.TopDomains, err = load("domain"); err != nil {
		return querylog.Summary{}, err
	}
	if result.TopClients, err = load("client_ip"); err != nil {
		return querylog.Summary{}, err
	}
	return result, nil
}
