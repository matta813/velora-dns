// Package backup provides backup status and verification functionality.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/api"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
)

type Manager struct {
	db             *database.Store
	databasePath   string
	mu             sync.RWMutex
	config         config.Config
	version        string
	lastBackupTime time.Time
	lastBackupSize int64
}

func (m *Manager) SetConfig(c config.Config, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = c.Clone()
	m.version = version
}

func NewManager(db *database.Store, databasePath string) *Manager {
	return &Manager{db: db, databasePath: databasePath}
}

func (m *Manager) BackupStatus() (*api.BackupStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := &api.BackupStatus{DatabasePath: m.databasePath, VerificationState: "unknown", Supported: m.db != nil && m.db.Driver() == "sqlite"}
	if !m.lastBackupTime.IsZero() {
		last := m.lastBackupTime
		status.LastBackupTime = &last
		status.LastBackupSize = m.lastBackupSize
		status.BackupAge = time.Since(m.lastBackupTime).Round(time.Second).String()
		status.VerificationState = "unverified"
	}
	return status, nil
}

func (m *Manager) resolveBackupPath(inputPath string) (string, error) {
	trimmed := strings.TrimSpace(inputPath)
	if trimmed == "" {
		return "", errors.New("backup path is required")
	}
	if filepath.IsAbs(trimmed) || strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "..") {
		return "", errors.New("backup path must be a file name without directory components")
	}

	baseDir, err := filepath.Abs(filepath.Dir(m.databasePath))
	if err != nil {
		return "", fmt.Errorf("resolve backup base directory: %w", err)
	}

	if trimmed == "." || trimmed == string(filepath.Separator) {
		return "", errors.New("invalid backup path")
	}

	return filepath.Join(baseDir, trimmed), nil
}

func (m *Manager) VerifyBackup(path string) (*api.BackupVerification, error) {
	validatedPath, err := m.resolveBackupPath(path)
	if err != nil {
		return &api.BackupVerification{
			Valid: false,
			Error: fmt.Sprintf("invalid backup path: %v", err),
		}, nil
	}

	stat, err := os.Lstat(validatedPath)
	if err != nil {
		return &api.BackupVerification{
			Valid: false,
			Error: fmt.Sprintf("backup file not found: %v", err),
		}, nil
	}
	if stat.Mode()&os.ModeSymlink != 0 || !stat.Mode().IsRegular() {
		return &api.BackupVerification{
			Valid: false,
			Error: "backup path must refer to a regular file",
		}, nil
	}

	db, err := sql.Open("sqlite", "file:"+validatedPath+"?mode=ro")
	if err != nil {
		return &api.BackupVerification{
			Valid: false,
			Error: fmt.Sprintf("open backup database: %v", err),
		}, nil
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return &api.BackupVerification{
			Valid: false,
			Error: fmt.Sprintf("ping backup database: %v", err),
		}, nil
	}

	var schemaVersion int
	err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&schemaVersion)
	if err != nil {
		return &api.BackupVerification{
			Valid: false,
			Error: fmt.Sprintf("read schema version: %v", err),
		}, nil
	}

	var recordCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM query_log").Scan(&recordCount)
	if err != nil {
		recordCount = 0
	}

	return &api.BackupVerification{
		Valid:         true,
		SchemaVersion: schemaVersion,
		RecordCount:   recordCount,
	}, nil
}
