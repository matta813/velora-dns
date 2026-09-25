package api

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/matta813/velora-dns/internal/config"
)

type DiagnosticComponent struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type DiagnosticReport struct {
	GeneratedAt     time.Time             `json:"generated_at"`
	Version         Version               `json:"version"`
	OS              string                `json:"os"`
	Architecture    string                `json:"architecture"`
	UptimeSeconds   int64                 `json:"uptime_seconds"`
	State           string                `json:"state"`
	Components      []DiagnosticComponent `json:"components"`
	UpstreamCount   int                   `json:"upstream_count"`
	QueryLogEnabled bool                  `json:"query_log_enabled"`
}

func diagnosticReport(ctx context.Context, d Dependencies, cfg config.Config) DiagnosticReport {
	report := DiagnosticReport{
		GeneratedAt: time.Now().UTC(), Version: d.Version,
		OS: runtime.GOOS, Architecture: runtime.GOARCH,
		UptimeSeconds: int64(time.Since(d.Started).Seconds()),
		State:         "healthy", Components: make([]DiagnosticComponent, 0, 6),
		UpstreamCount: len(cfg.DNS.Upstreams), QueryLogEnabled: cfg.QueryLog.Enabled,
	}
	add := func(name, state, detail string) {
		report.Components = append(report.Components, DiagnosticComponent{Name: name, State: state, Detail: detail})
		if state == "failed" {
			report.State = "failed"
		} else if state == "degraded" && report.State == "healthy" {
			report.State = "degraded"
		}
	}
	if d.DNS.Ready() {
		add("dns", "healthy", "DNS listeners are ready")
	} else {
		add("dns", "failed", "DNS listeners are not ready")
	}
	checkCtx, cancel := context.WithTimeout(ctx, time.Second)
	storageReady := d.Database.Ping(checkCtx) == nil
	cancel()
	if storageReady {
		add("storage", "healthy", "Management storage is reachable")
	} else {
		add("storage", "failed", "Management storage is unavailable")
	}
	if cfg.Validate() == nil {
		add("configuration", "healthy", "Current configuration is valid")
	} else {
		add("configuration", "degraded", "Current configuration fails validation")
	}
	cacheStats := d.Cache.Stats()
	add("cache", "healthy", fmt.Sprintf("%d of %d cache slots in use", cacheStats.Entries, cacheStats.Capacity))
	if cfg.QueryLog.Enabled {
		add("query_log", "healthy", "Query logging is enabled")
	} else {
		add("query_log", "healthy", "Query logging is disabled by configuration")
	}
	if d.Update != nil {
		updateCtx, cancel := context.WithTimeout(ctx, time.Second)
		health, err := d.Update.Health(updateCtx)
		cancel()
		if err != nil || !health.Ready {
			add("updater", "degraded", "Updater agent is unavailable")
		} else {
			add("updater", "healthy", "Updater agent is ready")
		}
	}
	return report
}

func registerDiagnostics(mux *http.ServeMux, d Dependencies, currentConfig func() config.Config) {
	mux.HandleFunc("GET /api/v1/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, diagnosticReport(r.Context(), d, currentConfig()))
	})
}
