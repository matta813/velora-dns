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

	"github.com/matta813/velora-dns/internal/deployment"
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
	mux.HandleFunc("GET /settings", a.handleSettings)
	mux.HandleFunc("PUT /settings", a.handleSettings)
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

func (a *Agent) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		settings, err := a.readSettings()
		if err != nil {
			a.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		a.writeJSON(w, http.StatusOK, deployment.Response{Settings: settings})
		return
	}
	var settings deployment.Settings
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&settings); err != nil || settings.Validate() != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid deployment settings"})
		return
	}
	if err := a.applySettings(settings); err != nil {
		a.config.Logger.Error("apply deployment settings", "error", err)
		a.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not apply deployment settings"})
		return
	}
	a.writeJSON(w, http.StatusOK, deployment.Response{Settings: settings, Applied: true})
}

func (a *Agent) readSettings() (deployment.Settings, error) {
	service, err := readEnvFile(a.config.ServiceEnv)
	if err != nil {
		return deployment.Settings{}, err
	}
	updater, err := readEnvFile(a.config.UpdaterEnv)
	if err != nil && !os.IsNotExist(err) {
		return deployment.Settings{}, err
	}
	settings := deployment.Settings{Channel: updater["VELORA_CHANNEL"], WebUIExposure: deployment.ExposureLocal, DNSExposure: deployment.ExposureLocal}
	if settings.Channel == "" {
		settings.Channel = a.config.Channel
	}
	if service["VELORA_HTTP_LISTEN"] == "0.0.0.0:8080" {
		settings.WebUIExposure = deployment.ExposureLAN
	}
	if service["VELORA_DNS_LISTEN"] == "0.0.0.0:53" {
		settings.DNSExposure = deployment.ExposureLAN
	}
	return settings, settings.Validate()
}

func (a *Agent) applySettings(settings deployment.Settings) error {
	service, err := readEnvFile(a.config.ServiceEnv)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if settings.WebUIExposure == deployment.ExposureLAN {
		service["VELORA_HTTP_LISTEN"] = "0.0.0.0:8080"
		service["VELORA_HTTP_ALLOWED_HOSTS"] = "*"
	} else {
		service["VELORA_HTTP_LISTEN"] = "127.0.0.1:8080"
		service["VELORA_HTTP_ALLOWED_HOSTS"] = "localhost,127.0.0.1,::1"
	}
	if settings.DNSExposure == deployment.ExposureLAN {
		service["VELORA_DNS_LISTEN"] = "0.0.0.0:53"
		service["VELORA_DNS_ALLOWED_CLIENTS"] = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8,::1/128"
	} else {
		service["VELORA_DNS_LISTEN"] = "127.0.0.1:53"
		service["VELORA_DNS_ALLOWED_CLIENTS"] = "127.0.0.0/8,::1/128"
	}
	updater, err := readEnvFile(a.config.UpdaterEnv)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updater["VELORA_CHANNEL"] = settings.Channel
	if err := writeEnvFile(a.config.ServiceEnv, service); err != nil {
		return err
	}
	if err := writeEnvFile(a.config.UpdaterEnv, updater); err != nil {
		return err
	}
	a.config.Channel = settings.Channel
	// The proxying API process is about to be restarted. Delay the restart long
	// enough for the successful response to cross the Unix-socket boundary.
	go func() {
		time.Sleep(250 * time.Millisecond)
		if err := exec.Command("systemctl", "restart", "velora-dns").Run(); err != nil {
			a.config.Logger.Error("restart after deployment settings change", "error", err)
		}
	}()
	return nil
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
	entry, err := a.manager.Begin(currentVersion, "latest", "systemd", a.config.Channel)
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
