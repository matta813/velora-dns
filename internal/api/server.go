// Package api exposes versioned operational endpoints behind one middleware boundary.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/querylog"
)

type Database interface{ Ping(context.Context) error }
type QueryStore interface {
	ListQueries(context.Context, querylog.Filter) ([]querylog.Entry, error)
	QuerySummary(context.Context, time.Time, time.Time, int) (querylog.Summary, error)
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
	Database       Database
	Zones          ZoneStore
	Filtering      BlocklistStore
	Queries        QueryStore
	Auth           AuthStore
	DNS            DNS
	Cache          *cache.Cache
	Metrics        *metrics.Metrics
	Config         config.Config
	ConfigPath     string
	Version        Version
	Started        time.Time
	TSIG           TSIGStore
	Update         UpdateStore
	Onboarding     OnboardingStore
	Backup         BackupStore
	Settings       SettingsStore
	RateLimit      *dns.RateLimitState
	UpstreamHealth *dns.UpstreamHealth
	ResetStats     func(context.Context) error
	Events         EventStore
	NotifyEvent    func(database.SystemEventInput)
	DHCP           DHCPStore
	Cluster        ClusterStore
	ApplyConfig    func(config.Config) error
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
	var (
		cfgMu         sync.RWMutex
		cfgWriteMu    sync.Mutex
		currentConfig = d.Config
	)
	mux := http.NewServeMux()
	if d.UpstreamHealth != nil {
		mux.HandleFunc("GET /api/v1/upstreams/health", func(w http.ResponseWriter, r *http.Request) {
			respond(w, 200, d.UpstreamHealth.Snapshot())
		})
	}
	registerDiagnostics(mux, d, func() config.Config {
		cfgMu.RLock()
		defer cfgMu.RUnlock()
		return currentConfig
	})
	if d.Auth != nil {
		registerAuth(mux, d.Auth)
		if d.Events != nil {
			registerEvents(mux, d.Events)
		}
	}
	capabilities := []string{"forwarding", "cache", "metrics"}
	if d.Zones != nil {
		registerZones(mux, d.Zones)
		registerSecondaryZones(mux, d.Zones)
		capabilities = append(capabilities, "local_zones")
	}
	if d.TSIG != nil {
		registerTSIG(mux, d.TSIG)
		capabilities = append(capabilities, "tsig")
	}
	if d.Filtering != nil {
		registerBlocklists(mux, d.Filtering)
		capabilities = append(capabilities, "blocklists")
	}
	if d.Queries != nil {
		registerQueries(mux, d.Queries, func() bool {
			cfgMu.RLock()
			defer cfgMu.RUnlock()
			return currentConfig.QueryLog.Enabled
		})
		capabilities = append(capabilities, "query_logging")
	}
	if d.Update != nil {
		registerUpdate(mux, d.Update, d.Version)
		capabilities = append(capabilities, "updates")
	}
	if d.Onboarding != nil {
		registerOnboarding(mux, d.Onboarding, d.Config)
	}
	if d.Backup != nil {
		registerBackup(mux, d.Backup, d.NotifyEvent)
	}
	if d.Settings != nil {
		registerSettings(mux, d.Settings, d.RateLimit)
	}
	if d.DHCP != nil {
		registerDHCP(mux, d.DHCP)
		capabilities = append(capabilities, "dhcp")
	}
	if d.Cluster != nil {
		registerCluster(mux, d.Cluster)
		capabilities = append(capabilities, "cluster")
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
		status := map[string]any{
			"ready":          d.DNS.Ready(),
			"uptime_seconds": time.Since(d.Started).Seconds(),
			"dns_listen":     d.DNS.Addresses(),
			"version":        d.Version,
			"capabilities":   capabilities,
		}
		if d.Cluster != nil {
			nodes, err := d.Cluster.ListNodes(r.Context())
			if err == nil {
				status["cluster_nodes"] = len(nodes)
				status["cluster_healthy"] = len(nodes) > 0
			}
		}
		respond(w, 200, status)
	})
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, d.Version) })
	mux.HandleFunc("GET /api/v1/stats", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, d.Metrics.Snapshot()) })
	mux.HandleFunc("POST /api/v1/stats/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			failure(w, 415, "unsupported_media_type", "Use application/json")
			return
		}
		if d.ResetStats == nil {
			failure(w, 503, "statistics_unavailable", "Statistics reset is unavailable")
			return
		}
		if err := d.ResetStats(r.Context()); err != nil {
			failure(w, 503, "statistics_reset_failed", "Statistics could not be reset")
			return
		}
		respond(w, 200, d.Metrics.Snapshot())
	})
	mux.HandleFunc("GET /api/v1/cache", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, d.Cache.Stats()) })
	mux.HandleFunc("GET /api/v1/cache/entries", func(w http.ResponseWriter, r *http.Request) {
		limit, offset := 100, 0
		if raw := r.URL.Query().Get("limit"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 200 {
				failure(w, 400, "invalid_limit", "Limit must be between 1 and 200")
				return
			}
			limit = value
		}
		if raw := r.URL.Query().Get("offset"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				failure(w, 400, "invalid_offset", "Offset must be non-negative")
				return
			}
			offset = value
		}
		respond(w, 200, d.Cache.ListEntries(limit, offset))
	})
	mux.HandleFunc("DELETE /api/v1/cache", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			failure(w, 415, "unsupported_media_type", "Use application/json")
			return
		}
		d.Cache.Flush()
		respond(w, 200, d.Cache.Stats())
	})
	mux.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		cfgMu.RLock()
		cfg := currentConfig
		cfgMu.RUnlock()
		respond(w, 200, cfg)
	})
	mux.HandleFunc("PUT /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		notifyRollback := func(severity, message string) {
			if d.NotifyEvent != nil {
				d.NotifyEvent(database.SystemEventInput{Key: "configuration_rollback", Severity: severity, Title: "Configuration rollback", Message: message, Link: "/settings", Visibility: "admin"})
			}
		}
		cfgWriteMu.Lock()
		defer cfgWriteMu.Unlock()
		if d.ConfigPath == "" {
			failure(w, 500, "config_path_missing", "Config path not configured")
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			failure(w, 415, "unsupported_media_type", "Use application/json")
			return
		}
		cfgMu.RLock()
		newConfig := currentConfig.Clone()
		cfgMu.RUnlock()
		if !readJSON(w, r, &newConfig) {
			return
		}
		if err := newConfig.Validate(); err != nil {
			failure(w, 400, "invalid_config", err.Error())
			return
		}
		if d.ApplyConfig != nil {
			if err := d.ApplyConfig(newConfig); err != nil {
				var restart *config.RestartRequiredError
				if errors.As(err, &restart) {
					failure(w, 409, "config_requires_restart", err.Error())
					return
				}
				if rollbackErr := d.ApplyConfig(currentConfig); rollbackErr != nil {
					notifyRollback("critical", "A configuration apply and its rollback both failed. Check service readiness.")
					failure(w, 500, "config_rollback_failed", fmt.Sprintf("Apply failed: %v; rollback failed: %v", err, rollbackErr))
					return
				}
				notifyRollback("warning", "A configuration apply failed and the previous settings were restored.")
				failure(w, 500, "config_apply_failed", err.Error())
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		ready := d.DNS.Ready() && d.Database.Ping(ctx) == nil
		cancel()
		if !ready {
			if d.ApplyConfig != nil {
				if err := d.ApplyConfig(currentConfig); err != nil {
					notifyRollback("critical", "Readiness failed and the previous configuration could not be restored.")
					failure(w, 500, "config_rollback_failed", fmt.Sprintf("Readiness failed; rollback failed: %v", err))
					return
				}
			}
			notifyRollback("warning", "A configuration change failed readiness and the previous settings were restored.")
			failure(w, 503, "config_not_ready", "Services are not ready after applying configuration")
			return
		}
		if err := newConfig.Save(d.ConfigPath); err != nil {
			if d.ApplyConfig != nil {
				if rollbackErr := d.ApplyConfig(currentConfig); rollbackErr != nil {
					notifyRollback("critical", "Configuration storage failed and the previous runtime settings could not be restored.")
					failure(w, 500, "config_rollback_failed", fmt.Sprintf("Save failed: %v; rollback failed: %v", err, rollbackErr))
					return
				}
				notifyRollback("warning", "Configuration storage failed and the previous runtime settings were restored.")
			}
			failure(w, 500, "config_save_failed", err.Error())
			return
		}
		cfgMu.Lock()
		currentConfig = newConfig
		cfgMu.Unlock()
		respond(w, 200, map[string]string{"status": "saved", "message": "Configuration saved and applied live where supported."})
	})
	mux.Handle("GET /metrics", d.Metrics.Handler())
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { failure(w, 404, "not_found", "Endpoint not found") })
	mux.Handle("/", web(func() string {
		cfgMu.RLock()
		defer cfgMu.RUnlock()
		return currentConfig.HTTP.WebDir
	}))
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
		cfgMu.RLock()
		allowedHosts := currentConfig.HTTP.AllowedHosts
		cfgMu.RUnlock()
		allowed := false
		for _, candidate := range allowedHosts {
			if candidate == "*" || strings.EqualFold(host, candidate) {
				allowed = true
				break
			}
		}
		if !allowed {
			failure(w, 403, "forbidden_host", "Host is not allowed")
			return
		}
		if expected, ok := map[string]string{"/health": "GET", "/ready": "GET", "/metrics": "GET", "/api/v1/status": "GET", "/api/v1/version": "GET", "/api/v1/stats": "GET", "/api/v1/config": "GET, PUT", "/api/v1/cache": "GET, DELETE"}[r.URL.Path]; ok {
			methodAllowed := r.Method == "GET" || r.Method == "HEAD" || (r.URL.Path == "/api/v1/cache" && r.Method == "DELETE") || (r.URL.Path == "/api/v1/config" && r.Method == "PUT")
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
		if d.Auth != nil && (strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/metrics") && r.URL.Path != "/api/v1/auth/login" {
			authenticated, csrf, ok := authenticate(r, d.Auth)
			if !ok {
				failure(w, http.StatusUnauthorized, "authentication_required", "Sign in to access management")
				return
			}
			r = authenticated
			user := r.Context().Value(authContextKey{}).(database.User)
			token, isToken := r.Context().Value(tokenContextKey{}).(database.APIToken)
			tokenScopes := token.Scopes
			unsafe := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
			if (strings.HasPrefix(r.URL.Path, "/api/v1/users") || r.URL.Path == "/api/v1/audit" || r.URL.Path == "/api/v1/stats/reset" || r.URL.Path == "/api/v1/backup/create") && (user.Role != "admin" || (isToken && !hasScope(tokenScopes, "admin"))) {
				failure(w, http.StatusForbidden, "insufficient_role", "Admin role required")
				return
			}
			markRead := strings.HasPrefix(r.URL.Path, "/api/v1/events/") && strings.HasSuffix(r.URL.Path, "/read") && r.Method == http.MethodPost
			if unsafe && (!markRead && user.Role == "viewer" || (isToken && !hasScope(tokenScopes, "write") && !hasScope(tokenScopes, "admin"))) {
				failure(w, http.StatusForbidden, "insufficient_role", "Viewer role is read-only")
				return
			}
			if unsafe && !isToken && !validCSRF(r.Header.Get("X-CSRF-Token"), csrf) {
				failure(w, http.StatusForbidden, "invalid_csrf", "Valid CSRF token required")
				return
			}
			if unsafe {
				id, err := d.Auth.BeginAudit(r.Context(), user.ID, user.Role, r.Method, r.URL.Path)
				if err != nil {
					failure(w, http.StatusServiceUnavailable, "audit_unavailable", "Audit event could not be recorded")
					return
				}
				tracked := &auditResponseWriter{ResponseWriter: w, status: http.StatusOK}
				w = tracked
				defer func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					_ = d.Auth.CompleteAudit(ctx, id, tracked.status)
				}()
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

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseWriter) Write(data []byte) (int, error) {
	return w.ResponseWriter.Write(data)
}

func (w *auditResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func hasScope(scopes, wanted string) bool {
	for _, scope := range strings.Split(scopes, ",") {
		if scope == wanted {
			return true
		}
	}
	return false
}
