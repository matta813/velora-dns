package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/app"
	"github.com/matta813/velora-dns/internal/backup"
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
	inspectBundle := flag.String("inspect-backup", "", "Inspect an encrypted backup without restoring")
	restoreBundle := flag.String("restore-backup", "", "Restore an encrypted backup while the service is stopped")
	restoreDatabase := flag.String("restore-database", "", "Target SQLite database path for offline restore")
	passphraseEnv := flag.String("backup-passphrase-env", "VELORA_BACKUP_PASSPHRASE", "Environment variable containing the backup passphrase")
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
	if *inspectBundle != "" || *restoreBundle != "" {
		passphrase := os.Getenv(*passphraseEnv)
		if passphrase == "" {
			return errors.New("backup passphrase environment variable is empty")
		}
		if *inspectBundle != "" && *restoreBundle != "" {
			return fmt.Errorf("inspect and restore are mutually exclusive")
		}
		if *inspectBundle != "" {
			metadata, err := backup.InspectBundle(context.Background(), *inspectBundle, passphrase)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(metadata)
		}
		if *path == "" || *restoreDatabase == "" {
			return fmt.Errorf("-config and -restore-database are required for restore")
		}
		metadata, safety, err := backup.RestoreOffline(context.Background(), *restoreBundle, passphrase, *path, *restoreDatabase)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "restored Velora backup format %d; safety copy: %s\n", metadata.FormatVersion, safety)
		return err
	}
	c, err := config.Load(*path)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return app.Run(ctx, c, *path, logging.New(os.Stdout, c.LogLevel), api.Version{Version: version, Commit: commit, Built: built})
}
