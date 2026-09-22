// Command velora-updater is a root-owned systemd update agent for native installations.
// It accepts only a fixed update request over a Unix socket, resolves a published GitHub
// Release, verifies the declared checksum, installs the matching bundle atomically and
// restarts Velora only after validation.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"
	"time"
)

const (
	defaultSocketPath   = "/run/velora-updater.sock"
	defaultStateFile    = "/var/lib/velora/update-state.json"
	defaultBinaryPath   = "/opt/velora/velora-dns"
	defaultWebDir       = "/opt/velora/web"
	defaultBackupSuffix = ".prev"
	socketPermissions   = 0660
)

type UpdateRequest struct {
	Action string `json:"action"`
}

type UpdateResponse struct {
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type AgentConfig struct {
	SocketPath   string
	StateFile    string
	BinaryPath   string
	WebDir       string
	Repository   string
	ReadinessURL string
	Channel      string
	Timeout      time.Duration
	Logger       *slog.Logger
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	config := AgentConfig{
		SocketPath:   envOrDefault("VELORA_UPDATER_SOCKET", defaultSocketPath),
		StateFile:    envOrDefault("VELORA_UPDATE_STATE_FILE", defaultStateFile),
		BinaryPath:   envOrDefault("VELORA_BINARY_PATH", defaultBinaryPath),
		WebDir:       envOrDefault("VELORA_WEB_DIR", defaultWebDir),
		Repository:   envOrDefault("VELORA_REPOSITORY", "matta813/velora-dns"),
		ReadinessURL: envOrDefault("VELORA_READINESS_URL", "http://127.0.0.1:8080/ready"),
		Channel:      envOrDefault("VELORA_CHANNEL", "stable"),
		Timeout:      10 * time.Minute,
		Logger:       logger,
	}

	if os.Getuid() != 0 {
		logger.Error("velora-updater must run as root")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	agent, err := NewAgent(config)
	if err != nil {
		logger.Error("failed to create agent", "error", err)
		os.Exit(1)
	}

	if err := agent.Run(ctx); err != nil {
		logger.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func veloraGID() (int, error) {
	group, err := user.LookupGroup("velora")
	if err != nil {
		return 0, err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil || gid < 0 {
		return 0, fmt.Errorf("invalid velora group ID %q", group.Gid)
	}
	return gid, nil
}
