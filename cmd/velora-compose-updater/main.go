// Command velora-compose-updater is a host-side Compose update agent.
// It resolves an immutable release tag and image digest from GitHub Releases,
// pulls that exact GHCR image, updates the deployment and verifies readiness.
// It must never use a floating latest tag.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"
	"time"
)

const (
	defaultComposeProject = "velora-dns"
	defaultServiceName    = "velora"
	defaultReadinessURL   = "http://127.0.0.1:8080/ready"
	defaultStateFile      = "/var/lib/velora/update-state.json"
	defaultSocketPath     = "/run/velora-compose-updater.sock"
)

type AgentConfig struct {
	ComposePath    string
	ComposeDir     string
	ComposeProject string
	ServiceName    string
	Repository     string
	ReadinessURL   string
	StateFile      string
	VersionFile    string
	Channel        string
	Timeout        time.Duration
	Logger         *slog.Logger
	SocketPath     string
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	config := AgentConfig{
		ComposePath:    envOrDefault("VELORA_COMPOSE_PATH", "docker compose"),
		ComposeDir:     envOrDefault("VELORA_COMPOSE_DIR", ""),
		ComposeProject: envOrDefault("VELORA_COMPOSE_PROJECT", defaultComposeProject),
		ServiceName:    envOrDefault("VELORA_SERVICE_NAME", defaultServiceName),
		Repository:     envOrDefault("VELORA_REPOSITORY", "matta813/velora-dns"),
		ReadinessURL:   envOrDefault("VELORA_READINESS_URL", defaultReadinessURL),
		StateFile:      envOrDefault("VELORA_UPDATE_STATE_FILE", defaultStateFile),
		VersionFile:    envOrDefault("VELORA_UPDATE_VERSION_FILE", "/var/lib/velora/compose-version"),
		Channel:        envOrDefault("VELORA_CHANNEL", "stable"),
		Timeout:        10 * time.Minute,
		Logger:         logger,
		SocketPath:     envOrDefault("VELORA_UPDATER_SOCKET", defaultSocketPath),
	}

	if config.ComposeDir == "" {
		dir, err := os.Getwd()
		if err != nil {
			logger.Error("failed to get working directory", "error", err)
			os.Exit(1)
		}
		config.ComposeDir = dir
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

func veloraGID() int {
	group, err := user.LookupGroup("velora")
	if err != nil {
		return -1
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return -1
	}
	return gid
}
