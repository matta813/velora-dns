package api

import (
	"context"
	"github.com/matta813/velora-dns/internal/querylog"
	wire "github.com/miekg/dns"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

func registerQueries(mux *http.ServeMux, store QueryStore, enabled bool) {
	mux.HandleFunc("GET /api/v1/queries", func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			failure(w, 503, "query_logging_disabled", "Query logging is disabled")
			return
		}
		q := r.URL.Query()
		f := querylog.Filter{Domain: strings.ToLower(q.Get("domain")), Client: q.Get("client"), Type: strings.ToUpper(q.Get("type")), Source: q.Get("source"), Limit: 100}
		invalid := func() {
			failure(w, 400, "invalid_filter", "Use valid domain, client IP, query type, source, limit (1–500) and before ID filters")
		}
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 500 {
				invalid()
				return
			}
			f.Limit = n
		}
		if v := q.Get("before"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 1 {
				invalid()
				return
			}
			f.Before = n
		}
		if len(f.Domain) > 253 {
			invalid()
			return
		}
		if f.Client != "" {
			ip, err := netip.ParseAddr(f.Client)
			if err != nil {
				invalid()
				return
			}
			f.Client = ip.Unmap().String()
		}
		if f.Type != "" {
			if _, ok := wire.StringToType[f.Type]; !ok {
				invalid()
				return
			}
		}
		switch f.Source {
		case "", "cache", "local", "upstream", "blocked", "refused", "overload":
		default:
			invalid()
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		entries, err := store.ListQueries(ctx, f)
		if err != nil {
			failure(w, 503, "storage_unavailable", "Query log storage is unavailable")
			return
		}
		if len(entries) > 0 {
			w.Header().Set("X-Next-Before", strconv.FormatInt(entries[len(entries)-1].ID, 10))
		}
		respond(w, 200, entries)
	})
	mux.HandleFunc("GET /api/v1/query-stats", func(w http.ResponseWriter, r *http.Request) {
		if !enabled {
			failure(w, 503, "query_logging_disabled", "Query logging is disabled")
			return
		}
		windows := map[string]time.Duration{"1h": time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour}
		windowName := r.URL.Query().Get("window")
		if windowName == "" {
			windowName = "24h"
		}
		window, ok := windows[windowName]
		limit := 10
		if value := r.URL.Query().Get("limit"); value != "" {
			var err error
			limit, err = strconv.Atoi(value)
			if err != nil || limit < 1 || limit > 50 {
				ok = false
			}
		}
		if !ok {
			failure(w, 400, "invalid_filter", "Use window 1h, 24h or 7d and limit 1–50")
			return
		}
		end := time.Now().UTC()
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		summary, err := store.QuerySummary(ctx, end.Add(-window), end, limit)
		if err != nil {
			failure(w, 503, "storage_unavailable", "Query statistics storage is unavailable")
			return
		}
		respond(w, 200, summary)
	})
	mux.HandleFunc("/api/v1/query-stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "GET, HEAD")
		failure(w, 405, "method_not_allowed", "Method not allowed")
	})
	mux.HandleFunc("/api/v1/queries", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "GET, HEAD")
		failure(w, 405, "method_not_allowed", "Method not allowed")
	})
}
