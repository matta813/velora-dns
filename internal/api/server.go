// Package api exposes versioned operational endpoints behind one middleware boundary.
package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/filtering"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/querylog"
)

type Database interface{ Ping(context.Context) error }
type QueryStore interface {
	ListQueries(context.Context, database.QueryFilter) ([]querylog.Entry, error)
}
type DNS interface {
	Ready() bool
	Addresses() []string
}
type Version struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Built   string `json:"built"`
}
type Dependencies struct {
	Database  Database
	Zones     ZoneStore
	Filtering *filtering.Service
	Queries   QueryStore
	DNS       DNS
	Cache     *cache.Cache
	Metrics   *metrics.Metrics
	Config    config.Config
	Version   Version
	Started   time.Time
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}
func failure(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": Error{Code: code, Message: message}})
}
func New(d Dependencies) http.Handler {
	mux := http.NewServeMux()
	capabilities := []string{"forwarding", "cache", "metrics"}
	if d.Zones != nil {
		registerZones(mux, d.Zones)
		capabilities = append(capabilities, "local_zones")
	}
	if d.Filtering != nil {
		registerBlocklists(mux, d.Filtering)
		capabilities = append(capabilities, "blocklists")
	}
	if d.Queries != nil {
		registerQueries(mux, d.Queries)
		capabilities = append(capabilities, "query_logging")
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "alive"}) })
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if !d.DNS.Ready() || d.Database.Ping(ctx) != nil {
			failure(w, 503, "not_ready", "Required components are unavailable")
			return
		}
		respond(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"ready": d.DNS.Ready(), "uptime_seconds": time.Since(d.Started).Seconds(), "dns_listen": d.DNS.Addresses(), "version": d.Version, "capabilities": capabilities})
	})
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, d.Version) })
	mux.HandleFunc("GET /api/v1/stats", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, d.Metrics.Snapshot()) })
	mux.HandleFunc("GET /api/v1/cache", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, d.Cache.Stats()) })
	mux.HandleFunc("DELETE /api/v1/cache", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			failure(w, 415, "unsupported_media_type", "Use application/json")
			return
		}
		d.Cache.Flush()
		respond(w, 200, d.Cache.Stats())
	})
	mux.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, d.Config) })
	mux.Handle("GET /metrics", d.Metrics.Handler())
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { failure(w, 404, "not_found", "Endpoint not found") })
	mux.Handle("/", web(d.Config.HTTP.WebDir))
	slots := make(chan struct{}, 32)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		allowed := false
		for _, candidate := range d.Config.HTTP.AllowedHosts {
			if strings.EqualFold(host, candidate) {
				allowed = true
				break
			}
		}
		if !allowed {
			failure(w, 403, "forbidden_host", "Host is not allowed")
			return
		}
		if expected, ok := map[string]string{"/health": "GET", "/ready": "GET", "/metrics": "GET", "/api/v1/status": "GET", "/api/v1/version": "GET", "/api/v1/stats": "GET", "/api/v1/config": "GET", "/api/v1/cache": "GET, DELETE"}[r.URL.Path]; ok {
			methodAllowed := r.Method == "GET" || r.Method == "HEAD" || (r.URL.Path == "/api/v1/cache" && r.Method == "DELETE")
			if !methodAllowed {
				w.Header().Set("Allow", expected+", HEAD")
				failure(w, 405, "method_not_allowed", "Method not allowed")
				return
			}
		}
		if r.ContentLength > 1<<20 {
			failure(w, 413, "payload_too_large", "Request exceeds 1 MiB")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			failure(w, 403, "forbidden_origin", "Cross-site requests are not allowed")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				failure(w, 403, "forbidden_origin", "Origin is not allowed")
				return
			}
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			failure(w, 429, "busy", "Too many requests")
			return
		}
		mux.ServeHTTP(w, r)
	})
}
