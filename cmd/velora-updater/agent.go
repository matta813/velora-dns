package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/update"
)

type Agent struct {
	config  AgentConfig
	manager *update.Manager
	resolve func(context.Context, string) (update.Release, error)
}

func NewAgent(config AgentConfig) (*Agent, error) {
	manager := update.NewManager(update.Config{
		ReadinessTimeout:  5 * time.Minute,
		ReadinessInterval: 5 * time.Second,
		MaxHistory:        10,
		StateFile:         config.StateFile,
	})
	if err := manager.LoadState(); err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	return &Agent{config: config, manager: manager}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	if err := os.Remove(a.config.SocketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	listener, err := net.Listen("unix", a.config.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on socket: %w", err)
	}
	defer func() {
		listener.Close()
		os.Remove(a.config.SocketPath)
	}()
	if err := os.Chown(a.config.SocketPath, 0, veloraGID()); err != nil {
		a.config.Logger.Warn("failed to chown socket", "error", err)
	}
	if err := os.Chmod(a.config.SocketPath, socketPermissions); err != nil {
		a.config.Logger.Warn("failed to chmod socket", "error", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /update", a.handleUpdate)
	mux.HandleFunc("GET /status", a.handleStatus)
	mux.HandleFunc("GET /history", a.handleHistory)
	mux.HandleFunc("GET /check", a.handleCheck)
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Minute,
	}
	a.config.Logger.Info("agent started", "socket", a.config.SocketPath)
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		a.config.Logger.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("server: %w", err)
	}
}

func (a *Agent) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if a.manager.IsUpdating() {
		a.writeJSON(w, 409, UpdateResponse{Status: "busy", Error: "update already in progress"})
		return
	}
	var req UpdateRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		a.writeJSON(w, 400, UpdateResponse{Status: "error", Error: "invalid request"})
		return
	}
	if req.Action != "update" {
		a.writeJSON(w, 400, UpdateResponse{Status: "error", Error: "unknown action"})
		return
	}
	currentVersion := a.readCurrentVersion()
	release, err := a.resolveRelease(r.Context(), currentVersion)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, update.ErrNoEligibleRelease) {
			status = http.StatusConflict
		}
		a.writeJSON(w, status, UpdateResponse{Status: "error", Error: err.Error()})
		return
	}
	entry, err := a.manager.Begin(currentVersion, release.Version, "systemd", a.config.Channel)
	if err != nil {
		a.writeJSON(w, 409, UpdateResponse{Status: "busy", Error: err.Error()})
		return
	}
	_ = a.saveState()
	go a.executeUpdate(entry, release)
	a.writeJSON(w, 202, UpdateResponse{Status: "accepted", Version: entry.ToVersion})
}

func (a *Agent) handleStatus(w http.ResponseWriter, r *http.Request) {
	current := a.manager.Current()
	if current == nil {
		history := a.manager.History()
		lastState := "idle"
		if len(history) > 0 {
			lastState = string(history[len(history)-1].State)
		}
		status := update.Status{State: update.State(lastState), Installed: a.readCurrentVersion()}
		if len(history) > 0 {
			last := history[len(history)-1]
			status.LastCompleted = last.CompletedAt
			status.Error = last.Error
			status.RollbackUsed = last.RollbackUsed
			status.ReadinessOK = last.ReadinessOK
		}
		a.writeJSON(w, 200, status)
		return
	}
	a.writeJSON(w, 200, update.Status{State: current.State, Installed: a.readCurrentVersion(), FromVersion: current.FromVersion, ToVersion: current.ToVersion, Channel: current.Channel, StartedAt: current.StartedAt, Updating: true, Error: current.Error, RollbackUsed: current.RollbackUsed, ReadinessOK: current.ReadinessOK})
}

func (a *Agent) handleHistory(w http.ResponseWriter, _ *http.Request) {
	a.writeJSON(w, 200, a.manager.History())
}

func (a *Agent) handleCheck(w http.ResponseWriter, r *http.Request) {
	installed := a.readCurrentVersion()
	release, err := a.resolveRelease(r.Context(), installed)
	if errors.Is(err, update.ErrNoEligibleRelease) {
		a.writeJSON(w, 200, update.CheckResult{Installed: installed, Latest: installed, Channel: a.config.Channel, Architecture: runtime.GOOS + "/" + runtime.GOARCH})
		return
	}
	if err != nil {
		a.writeJSON(w, 502, UpdateResponse{Status: "error", Error: err.Error()})
		return
	}
	a.writeJSON(w, 200, update.CheckResult{Installed: installed, Latest: release.Version, UpdateAvailable: true, Channel: release.Channel, ReleaseDate: release.PublishedAt, ReleaseNotes: release.Notes, Architecture: release.Architecture, DownloadSize: release.DownloadSize})
}

func (a *Agent) resolver() update.Resolver {
	return update.Resolver{Repository: a.config.Repository, Channel: a.config.Channel}
}

func (a *Agent) resolveRelease(ctx context.Context, installed string) (update.Release, error) {
	if a.resolve != nil {
		return a.resolve(ctx, installed)
	}
	return a.resolver().Resolve(ctx, installed)
}

func (a *Agent) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (a *Agent) readCurrentVersion() string {
	data, err := os.ReadFile(filepath.Join(filepath.Dir(a.config.BinaryPath), "VERSION"))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}
