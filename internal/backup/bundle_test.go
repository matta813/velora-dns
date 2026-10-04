package backup

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
)

func TestEncryptedBackupRestoreOffline(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "velora.db")
	configPath := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.DatabasePath = dbPath
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, "sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser(ctx, "original", "long original password", "admin"); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(db, dbPath)
	manager.SetConfig(cfg, "test-version")
	bundle, metadata, err := manager.CreateBundle(ctx, "a sufficiently long passphrase")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(bundle)
	if metadata.FormatVersion != 1 || metadata.SchemaVersion < 1 {
		t.Fatalf("metadata: %+v", metadata)
	}
	if status, err := manager.BackupStatus(); err != nil || status.LastBackupTime == nil || status.LastBackupSize == 0 {
		t.Fatalf("backup status: %+v %v", status, err)
	}
	if _, err := InspectBundle(ctx, bundle, "wrong passphrase long"); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	tampered, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	tampered[len(tampered)/2] ^= 0x80
	tamperedPath := filepath.Join(dir, "tampered.vdns")
	if err := os.WriteFile(tamperedPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectBundle(ctx, tamperedPath, "a sufficiently long passphrase"); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("tampered bundle: %v", err)
	}
	if _, err := InspectBundle(ctx, bundle, "a sufficiently long passphrase"); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = database.Open(ctx, "sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUser(ctx, "later", "long second password", "viewer"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.LogLevel = "debug"
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := RestoreOffline(ctx, bundle, "wrong passphrase long", configPath, dbPath); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("wrong password changed install: %v", err)
	}
	db, err = database.Open(ctx, "sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	count, err := db.UserCount(ctx)
	if err != nil || count != 2 {
		t.Fatalf("invalid restore changed users: %d %v", count, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	restored, safety, err := RestoreOffline(ctx, bundle, "a sufficiently long passphrase", configPath, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if restored.SchemaVersion != metadata.SchemaVersion || safety == "" {
		t.Fatalf("restore metadata: %+v %q", restored, safety)
	}
	db, err = database.Open(ctx, "sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	count, err = db.UserCount(ctx)
	if err != nil || count != 1 {
		t.Fatalf("users after restore: %d %v", count, err)
	}
	loaded, err := config.Load(configPath)
	if err != nil || loaded.LogLevel != "info" {
		t.Fatalf("config after restore: %s %v", loaded.LogLevel, err)
	}
	if _, err := os.Stat(filepath.Join(safety, "database.sqlite")); err != nil {
		t.Fatalf("safety copy missing: %v", err)
	}
}

func TestInspectRejectsMissingRequiredDataAndNewerSchema(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "source.db")
	cfg := config.Default()
	cfg.DatabasePath = path
	db, err := database.Open(ctx, "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, "DROP TABLE zones"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = database.Open(ctx, "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(db, path)
	manager.SetConfig(cfg, "test")
	bundle, _, err := manager.CreateBundle(ctx, "a sufficiently long passphrase")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(bundle)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectBundle(ctx, bundle, "a sufficiently long passphrase"); err == nil {
		t.Fatal("bundle missing zones table accepted")
	}
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES(999)"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = database.Open(ctx, "sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	manager = NewManager(db, path)
	manager.SetConfig(cfg, "test")
	newer, _, err := manager.CreateBundle(ctx, "a sufficiently long passphrase")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(newer)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectBundle(ctx, newer, "a sufficiently long passphrase"); err == nil {
		t.Fatal("newer schema accepted")
	}
}

func TestOfflineRestoreRefusesRunningInstance(t *testing.T) {
	dir := t.TempDir()
	lock, err := AcquireInstanceLock(filepath.Join(dir, "velora.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, _, err := RestoreOffline(context.Background(), "missing", "a sufficiently long passphrase", filepath.Join(dir, "config.yaml"), filepath.Join(dir, "velora.db")); err == nil {
		t.Fatal("restore accepted while server lock held")
	}
}
