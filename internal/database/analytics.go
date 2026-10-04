package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/matta813/velora-dns/internal/querylog"
)

// AnalyticsRange describes one selectable window. Buckets group rows by a
// prefix of the stored RFC 3339 timestamp, which works the same in SQLite
// and PostgreSQL and keeps the result size fixed.
type AnalyticsRange struct {
	Window time.Duration
	Bucket time.Duration
	prefix int
}

var AnalyticsRanges = map[string]AnalyticsRange{
	"1h":  {Window: time.Hour, Bucket: time.Minute, prefix: 16},
	"24h": {Window: 24 * time.Hour, Bucket: time.Hour, prefix: 13},
	"7d":  {Window: 7 * 24 * time.Hour, Bucket: time.Hour, prefix: 13},
	"30d": {Window: 30 * 24 * time.Hour, Bucket: 24 * time.Hour, prefix: 10},
}

const failedCondition = "response_code='SERVFAIL'"

// QueryAnalytics aggregates retained history for one range ending at end.
func (s *Store) QueryAnalytics(ctx context.Context, name string, end time.Time) (querylog.Analytics, error) {
	spec, ok := AnalyticsRanges[name]
	if !ok {
		return querylog.Analytics{}, fmt.Errorf("unknown analytics range %q", name)
	}
	end = end.UTC()
	// Align the window to whole buckets so the first and last bars are comparable.
	last := end.Truncate(spec.Bucket)
	count := int(spec.Window / spec.Bucket)
	start := last.Add(-time.Duration(count-1) * spec.Bucket)
	startText, endText := start.Format(queryTimeFormat), end.Add(time.Nanosecond).Format(queryTimeFormat)
	p := s.placeholder
	result := querylog.Analytics{Range: name, WindowStart: start, WindowEnd: end, BucketSeconds: int64(spec.Bucket / time.Second), Series: make([]querylog.AnalyticsBucket, count)}
	for i := range result.Series {
		result.Series[i].Start = start.Add(time.Duration(i) * spec.Bucket)
	}
	window := fmt.Sprintf("occurred_at>=%s AND occurred_at<%s", p(1), p(2))
	counters := fmt.Sprintf("COUNT(*), COALESCE(SUM(CASE WHEN source='blocked' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN source='cache' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN %s THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN source='upstream' AND response_code<>'SERVFAIL' THEN duration_micros ELSE 0 END),0), COALESCE(SUM(CASE WHEN source='upstream' AND response_code<>'SERVFAIL' THEN 1 ELSE 0 END),0)", failedCondition)
	scanBucket := func(row interface{ Scan(...any) error }, bucket *querylog.AnalyticsBucket, extra ...any) error {
		var micros, timed int64
		if err := row.Scan(append(extra, &bucket.Total, &bucket.Blocked, &bucket.Cached, &bucket.Failed, &micros, &timed)...); err != nil {
			return err
		}
		if timed > 0 {
			bucket.AverageMilliseconds = float64(micros) / float64(timed) / 1000
		}
		return nil
	}
	if err := scanBucket(s.db.QueryRowContext(ctx, "SELECT "+counters+" FROM query_log WHERE "+window, startText, endText), &result.Totals); err != nil {
		return querylog.Analytics{}, err
	}
	result.Totals.Start = start
	var oldest sql.NullString
	if err := s.db.QueryRowContext(ctx, "SELECT MIN(occurred_at) FROM query_log").Scan(&oldest); err != nil {
		return querylog.Analytics{}, err
	}
	if oldest.Valid {
		if at, err := time.Parse(queryTimeFormat, oldest.String); err == nil {
			result.HistoryStart = &at
		}
	}
	if result.Totals.Total == 0 {
		result.QueryTypes, result.ResponseCodes, result.Sources, result.TopBlocked = []querylog.Ranking{}, []querylog.Ranking{}, []querylog.Ranking{}, []querylog.Ranking{}
		result.Upstreams = []querylog.UpstreamUsage{}
		return result, nil
	}
	bucketExpr := fmt.Sprintf("substr(occurred_at,1,%d)", spec.prefix)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("SELECT %s AS bucket, %s FROM query_log WHERE %s GROUP BY bucket", bucketExpr, counters, window), startText, endText)
	if err != nil {
		return querylog.Analytics{}, err
	}
	layout := queryTimeFormat[:spec.prefix]
	for rows.Next() {
		var key string
		var bucket querylog.AnalyticsBucket
		if err = scanBucket(rows, &bucket, &key); err != nil {
			_ = rows.Close()
			return querylog.Analytics{}, err
		}
		at, parseErr := time.Parse(layout, key)
		if parseErr != nil {
			continue
		}
		i := int(at.Sub(start) / spec.Bucket)
		if i >= 0 && i < count {
			bucket.Start = result.Series[i].Start
			result.Series[i] = bucket
		}
	}
	if err = rows.Close(); err != nil {
		return querylog.Analytics{}, err
	}
	ranking := func(column, filter string, limit int) ([]querylog.Ranking, error) {
		statement := fmt.Sprintf("SELECT %s, COUNT(*) AS frequency FROM query_log WHERE %s%s GROUP BY %s ORDER BY frequency DESC, %s ASC LIMIT %s", column, window, filter, column, column, p(3))
		rows, err := s.db.QueryContext(ctx, statement, startText, endText, limit)
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
	if result.QueryTypes, err = ranking("query_type", "", 12); err != nil {
		return querylog.Analytics{}, err
	}
	if result.ResponseCodes, err = ranking("response_code", "", 8); err != nil {
		return querylog.Analytics{}, err
	}
	if result.Sources, err = ranking("source", "", 8); err != nil {
		return querylog.Analytics{}, err
	}
	if result.TopBlocked, err = ranking("domain", " AND source='blocked'", 10); err != nil {
		return querylog.Analytics{}, err
	}
	upstreamSQL := fmt.Sprintf("SELECT upstream, COUNT(*) AS frequency, COALESCE(SUM(CASE WHEN %s THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN %s THEN 0 ELSE duration_micros END),0) FROM query_log WHERE %s AND source='upstream' AND upstream<>'' GROUP BY upstream ORDER BY frequency DESC, upstream ASC LIMIT 10", failedCondition, failedCondition, window)
	rows, err = s.db.QueryContext(ctx, upstreamSQL, startText, endText)
	if err != nil {
		return querylog.Analytics{}, err
	}
	defer func() { _ = rows.Close() }()
	result.Upstreams = []querylog.UpstreamUsage{}
	for rows.Next() {
		var usage querylog.UpstreamUsage
		var micros int64
		if err = rows.Scan(&usage.Address, &usage.Queries, &usage.Failed, &micros); err != nil {
			return querylog.Analytics{}, err
		}
		if answered := usage.Queries - usage.Failed; answered > 0 {
			usage.AverageMilliseconds = float64(micros) / float64(answered) / 1000
		}
		result.Upstreams = append(result.Upstreams, usage)
	}
	return result, rows.Err()
}
