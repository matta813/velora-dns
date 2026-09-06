package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/app"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/logging"
)

var version = "dev"
var commit = "unknown"
var built = "unknown"

func main() {
	if err := run(); err != nil {
		logging.New(os.Stderr, "error").Error("server failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "", "YAML configuration path (optional)")
	check := flag.String("healthcheck", "", "Check an HTTP readiness URL and exit")
	flag.Parse()
	if *check != "" {
		client := http.Client{Timeout: 3 * time.Second}
		response, err := client.Get(*check)
		if err != nil {
			return err
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("readiness returned HTTP %d", response.StatusCode)
		}
		return nil
	}
	c, err := config.Load(*path)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return app.Run(ctx, c, logging.New(os.Stdout, c.LogLevel), api.Version{Version: version, Commit: commit, Built: built})
}
