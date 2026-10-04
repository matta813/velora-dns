package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/querylog"
)

func TestQueryAnalyticsBucketsAndBreakdowns(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "velora.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	end := time.Date(2026, 9, 27, 12, 30, 0, 0, time.UTC)
	if a, err := store.QueryAnalytics(ctx, "24h", end); err != nil || a.Totals.Total != 0 || len(a.Series) != 24 || a.HistoryStart != nil || a.Upstreams == nil {
		t.Fatalf("empty: %v %+v", err, a)
	}
	at := func(ago time.Duration) time.Time { return end.Add(-ago) }
	entries := []querylog.Entry{
		{OccurredAt: at(10 * time.Minute), ClientIP: "192.0.2.1", Domain: "a.example.", Type: "A", Rcode: "NOERROR", Source: "upstream", Upstream: "1.1.1.1:53", Duration: 20 * time.Millisecond},
		{OccurredAt: at(10 * time.Minute), ClientIP: "192.0.2.1", Domain: "b.example.", Type: "AAAA", Rcode: "NOERROR", Source: "upstream", Upstream: "1.1.1.1:53", Duration: 40 * time.Millisecond},
		{OccurredAt: at(20 * time.Minute), ClientIP: "192.0.2.2", Domain: "ads.example.", Type: "A", Rcode: "NXDOMAIN", Source: "blocked"},
		{OccurredAt: at(2 * time.Hour), ClientIP: "192.0.2.2", Domain: "a.example.", Type: "A", Rcode: "NOERROR", Source: "cache", CacheHit: true},
		{OccurredAt: at(3 * time.Hour), ClientIP: "192.0.2.3", Domain: "c.example.", Type: "MX", Rcode: "SERVFAIL", Source: "upstream", Upstream: "9.9.9.9:53", Duration: 2 * time.Second},
		{OccurredAt: at(3 * 24 * time.Hour), ClientIP: "192.0.2.3", Domain: "old.example.", Type: "TXT", Rcode: "NOERROR", Source: "local"},
	}
	if err = store.WriteQueries(ctx, entries, end.Add(-40*24*time.Hour), 1000); err != nil {
		t.Fatal(err)
	}
	day, err := store.QueryAnalytics(ctx, "24h", end)
	if err != nil {
		t.Fatal(err)
	}
	if day.Totals.Total != 5 || day.Totals.Blocked != 1 || day.Totals.Cached != 1 || day.Totals.Failed != 1 || day.Totals.AverageMilliseconds != 30 {
		t.Fatalf("totals: %+v", day.Totals)
	}
	if day.BucketSeconds != 3600 || !day.Series[23].Start.Equal(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)) || day.Series[23].Total != 3 || day.Series[21].Cached != 1 || day.Series[20].Failed != 1 {
		t.Fatalf("series: %+v", day.Series[20:])
	}
	if day.QueryTypes[0].Value != "A" || day.QueryTypes[0].Count != 3 || len(day.TopBlocked) != 1 || day.TopBlocked[0].Value != "ads.example." {
		t.Fatalf("breakdowns: %+v %+v", day.QueryTypes, day.TopBlocked)
	}
	if len(day.Upstreams) != 2 || day.Upstreams[0].Address != "1.1.1.1:53" || day.Upstreams[0].AverageMilliseconds != 30 || day.Upstreams[1].Failed != 1 {
		t.Fatalf("upstreams: %+v", day.Upstreams)
	}
	if day.HistoryStart == nil || !day.HistoryStart.Equal(at(3*24*time.Hour)) {
		t.Fatalf("history start: %v", day.HistoryStart)
	}
	hour, err := store.QueryAnalytics(ctx, "1h", end)
	if err != nil || len(hour.Series) != 60 || hour.Totals.Total != 3 || hour.Series[49].Total != 2 {
		t.Fatalf("1h: %v %+v", err, hour.Totals)
	}
	month, err := store.QueryAnalytics(ctx, "30d", end)
	if err != nil || len(month.Series) != 30 || month.Totals.Total != 6 || month.Series[26].Total != 1 || month.Series[29].Total != 5 {
		t.Fatalf("30d: %v %+v", err, month.Totals)
	}
	if _, err = store.QueryAnalytics(ctx, "1y", end); err == nil {
		t.Fatal("unknown range must fail")
	}
}
