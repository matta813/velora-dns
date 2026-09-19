package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/update"
)

type Agent struct {
	config  AgentConfig
	manager *update.Manager
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
	entry, err := a.manager.Begin(currentVersion, "latest", "systemd")
	if err != nil {
		a.writeJSON(w, 409, UpdateResponse{Status: "busy", Error: err.Error()})
		return
	}
	go a.executeUpdate(entry)
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
		a.writeJSON(w, 200, map[string]string{"state": lastState})
		return
	}
	a.writeJSON(w, 200, current)
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
