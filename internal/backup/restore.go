package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/filtering"
	"github.com/matta813/velora-dns/internal/zones"
	_ "modernc.org/sqlite"
)

func verifySQLite(ctx context.Context, path string, expectedSchema int) error {
	connection, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	var result string
	if err := connection.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil || result != "ok" {
		return fmt.Errorf("backup database integrity check failed: %s (%v)", result, err)
	}
	var schema int
	if err := connection.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&schema); err != nil {
		return err
	}
	if schema != expectedSchema {
		return fmt.Errorf("backup schema version mismatch: metadata %d, database %d", expectedSchema, schema)
	}
	return nil
}

func validateStagedBundle(ctx context.Context, stage, databasePath string) (Metadata, config.Config, error) {
	metadataBytes, err := os.ReadFile(filepath.Join(stage, "metadata.json"))
	if err != nil {
		return Metadata{}, config.Config{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return Metadata{}, config.Config{}, ErrInvalidBundle
	}
	latest, err := database.LatestSchemaVersion()
	if err != nil {
		return Metadata{}, config.Config{}, err
	}
	if metadata.SchemaVersion > latest {
		return Metadata{}, config.Config{}, fmt.Errorf("backup schema %d is newer than supported schema %d", metadata.SchemaVersion, latest)
	}
	if err := verifySQLite(ctx, filepath.Join(stage, "database.sqlite"), metadata.SchemaVersion); err != nil {
		return Metadata{}, config.Config{}, err
	}
	configBytes, err := os.ReadFile(filepath.Join(stage, "config.yaml"))
	if err != nil {
		return Metadata{}, config.Config{}, err
	}
	cfg, err := config.Parse(configBytes, func(string) (string, bool) { return "", false })
	if err != nil {
		return Metadata{}, config.Config{}, err
	}
	if cfg.DatabaseDriver != "sqlite" {
		return Metadata{}, config.Config{}, ErrInvalidBundle
	}
	cfg.DatabasePath = databasePath
	if err := cfg.Validate(); err != nil {
		return Metadata{}, config.Config{}, err
	}
	if err := checkApplicationState(ctx, filepath.Join(stage, "database.sqlite"), cfg); err != nil {
		return Metadata{}, config.Config{}, err
	}
	return metadata, cfg, nil
}

func checkApplicationState(ctx context.Context, path string, cfg config.Config) error {
	store, err := database.Open(ctx, "sqlite", path)
	if err != nil {
		return err
	}
	_, zoneErr := zones.New(ctx, store, nil)
	rules := make([]filtering.Rule, 0, len(cfg.Filtering.Blocklist)+len(cfg.Filtering.Allowlist))
	for _, domain := range cfg.Filtering.Blocklist {
		rules = append(rules, filtering.Rule{Domain: domain, Wildcard: true, Action: filtering.Block})
	}
	for _, domain := range cfg.Filtering.Allowlist {
		rules = append(rules, filtering.Rule{Domain: domain, Action: filtering.Allow})
	}
	_, filterErr := filtering.NewService(ctx, store, rules)
	return errors.Join(zoneErr, filterErr, store.Close())
}

// InspectBundle verifies authentication, schema and configuration without
// modifying the installation. It never returns decrypted secrets.
func InspectBundle(ctx context.Context, path, passphrase string) (Metadata, error) {
	stage, err := os.MkdirTemp("", "velora-inspect-*")
	if err != nil {
		return Metadata{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if _, err := readBundle(path, passphrase, stage); err != nil {
		return Metadata{}, err
	}
	metadata, _, err := validateStagedBundle(ctx, stage, filepath.Join(stage, "installation.db"))
	return metadata, err
}

func copyPrivate(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func checkpointSQLite(ctx context.Context, path string) error {
	connection, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	_, err = connection.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// RestoreOffline replaces the installation only after the bundle is fully
// authenticated and checked. The server must be stopped; its instance lock
// prevents replacing files still used by the running resolver.
func RestoreOffline(ctx context.Context, bundlePath, passphrase, configPath, databasePath string) (Metadata, string, error) {
	if configPath == "" || databasePath == "" {
		return Metadata{}, "", errors.New("config and database paths are required")
	}
	for _, path := range []string{configPath, databasePath} {
		info, err := os.Lstat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return Metadata{}, "", err
		}
		if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
			return Metadata{}, "", fmt.Errorf("restore target is not a regular file: %s", path)
		}
	}
	lock, err := AcquireInstanceLock(databasePath)
	if err != nil {
		return Metadata{}, "", err
	}
	defer func() { _ = lock.Close() }()
	stage, err := os.MkdirTemp(filepath.Dir(databasePath), ".velora-restore-stage-*")
	if err != nil {
		return Metadata{}, "", err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if _, err := readBundle(bundlePath, passphrase, stage); err != nil {
		return Metadata{}, "", err
	}
	metadata, cfg, err := validateStagedBundle(ctx, stage, databasePath)
	if err != nil {
		return Metadata{}, "", err
	}
	configStage, err := os.CreateTemp(filepath.Dir(configPath), ".velora-restore-config-*")
	if err != nil {
		return Metadata{}, "", err
	}
	configStagePath := configStage.Name()
	_ = configStage.Close()
	_ = os.Remove(configStagePath)
	defer func() { _ = os.Remove(configStagePath) }()
	if err := cfg.Save(configStagePath); err != nil {
		return Metadata{}, "", err
	}
	if err := os.Chmod(configStagePath, 0o600); err != nil {
		return Metadata{}, "", err
	}
	if _, err := os.Stat(databasePath); err == nil {
		if err := checkpointSQLite(ctx, databasePath); err != nil {
			return Metadata{}, "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Metadata{}, "", err
	}
	safety := filepath.Join(filepath.Dir(databasePath), fmt.Sprintf("velora-restore-safety-%s", time.Now().UTC().Format("20060102T150405.000000000")))
	if err := os.Mkdir(safety, 0o700); err != nil {
		return Metadata{}, "", err
	}
	previousDB, previousConfig := false, false
	if _, err := os.Stat(databasePath); err == nil {
		if err := copyPrivate(databasePath, filepath.Join(safety, "database.sqlite")); err != nil {
			return Metadata{}, safety, err
		}
		previousDB = true
	}
	if _, err := os.Stat(configPath); err == nil {
		if err := copyPrivate(configPath, filepath.Join(safety, "config.yaml")); err != nil {
			return Metadata{}, safety, err
		}
		previousConfig = true
	}
	rollback := func(cause error) (Metadata, string, error) {
		_ = os.Remove(databasePath)
		_ = os.Remove(configPath)
		var restoreErr error
		if previousDB {
			restoreErr = errors.Join(restoreErr, copyPrivate(filepath.Join(safety, "database.sqlite"), databasePath))
		}
		if previousConfig {
			restoreErr = errors.Join(restoreErr, copyPrivate(filepath.Join(safety, "config.yaml"), configPath))
		}
		return Metadata{}, safety, errors.Join(cause, restoreErr)
	}
	if err := os.Remove(databasePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Metadata{}, safety, err
	}
	_ = os.Remove(databasePath + "-wal")
	_ = os.Remove(databasePath + "-shm")
	if err := os.Rename(filepath.Join(stage, "database.sqlite"), databasePath); err != nil {
		return rollback(err)
	}
	if err := os.Rename(configStagePath, configPath); err != nil {
		return rollback(err)
	}
	if err := checkApplicationState(ctx, databasePath, cfg); err != nil {
		return rollback(err)
	}
	return metadata, safety, nil
}
