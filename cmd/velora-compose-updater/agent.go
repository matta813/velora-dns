package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
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
	socketPath := "/run/velora-compose-updater.sock"
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on socket: %w", err)
	}
	defer func() {
		listener.Close()
		os.Remove(socketPath)
	}()
	_ = os.Chmod(socketPath, 0660)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /update", a.handleUpdate)
	mux.HandleFunc("GET /status", a.handleStatus)
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Minute,
	}
	a.config.Logger.Info("compose updater started", "socket", socketPath)
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

type updateRequest struct {
	Action string `json:"action"`
}

type updateResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (a *Agent) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if a.manager.IsUpdating() {
		a.writeJSON(w, 409, updateResponse{Status: "busy", Error: "update already in progress"})
		return
	}
	var req updateRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		a.writeJSON(w, 400, updateResponse{Status: "error", Error: "invalid request"})
		return
	}
	if req.Action != "update" {
		a.writeJSON(w, 400, updateResponse{Status: "error", Error: "unknown action"})
		return
	}
	currentVersion := a.readCurrentVersion()
	entry, err := a.manager.Begin(currentVersion, "latest", "compose", a.config.Channel)
	if err != nil {
		a.writeJSON(w, 409, updateResponse{Status: "busy", Error: err.Error()})
		return
	}
	go a.executeUpdate(entry)
	a.writeJSON(w, 202, updateResponse{Status: "accepted", Version: entry.ToVersion})
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
	f, err := os.Open(filepath.Join(a.config.ComposeDir, "RELEASE"))
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	return strings.TrimSpace(string(buf[:n]))
}

func (a *Agent) composeArgs(extra ...string) []string {
	parts := strings.Fields(a.config.ComposePath)
	parts = append(parts, "-p", a.config.ComposeProject)
	parts = append(parts, extra...)
	return parts
}

func (a *Agent) runCompose(ctx context.Context, args ...string) (string, error) {
	composeArgs := a.composeArgs(args...)
	cmd := exec.CommandContext(ctx, composeArgs[0], composeArgs[1:]...)
	cmd.Dir = a.config.ComposeDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
