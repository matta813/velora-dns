package backup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/backupmeta"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"gopkg.in/yaml.v3"
)

// Online restore keeps the running server untouched until a restart:
//
//  1. Inspect: an uploaded bundle is decrypted and fully validated into a
//     private staging directory next to the database.
//  2. Schedule: the staged files become the single pending restore and the
//     server restarts.
//  3. Apply: on start, before the database is opened, the pending restore
//     replaces the database and configuration, keeping a safety copy.
//  4. Confirm: once the restarted server is ready the restore is marked
//     completed. If a start with restored data never becomes ready, the next
//     start rolls back to the safety copy automatically.
const (
	stagingDir    = ".velora-restore-staging"
	pendingDir    = ".velora-restore-pending"
	resultFile    = "velora-restore-result.json"
	stageLifetime = 30 * time.Minute
)

// Shared with the management API.
type (
	Summary      = backupmeta.Summary
	Inspection   = backupmeta.Inspection
	RestoreState = backupmeta.RestoreState
)

const MaxUploadBytes = backupmeta.MaxUploadBytes

var ErrStageNotFound = backupmeta.ErrStageNotFound

var tokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func dataDir(databasePath string) string { return filepath.Dir(databasePath) }

func newToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// StageUpload validates an uploaded bundle without touching the live
// installation and keeps the decrypted, checked files for StageRestore.
func (m *Manager) StageUpload(ctx context.Context, upload io.Reader, passphrase string) (Inspection, error) {
	m.mu.RLock()
	current, version := m.config.Clone(), m.version
	m.mu.RUnlock()
	if current.DatabaseDriver != "sqlite" || m.databasePath == "" {
		return Inspection{}, errors.New("online restore requires an SQLite installation")
	}
	root := filepath.Join(dataDir(m.databasePath), stagingDir)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Inspection{}, err
	}
	cleanupStages(root, time.Now())
	token, err := newToken()
	if err != nil {
		return Inspection{}, err
	}
	stage := filepath.Join(root, token)
	if err := os.Mkdir(stage, 0o700); err != nil {
		return Inspection{}, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	bundlePath := filepath.Join(stage, "upload.vdns")
	output, err := os.OpenFile(bundlePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Inspection{}, err
	}
	written, copyErr := io.Copy(output, io.LimitReader(upload, MaxUploadBytes+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		return Inspection{}, errors.Join(copyErr, closeErr)
	}
	if written > MaxUploadBytes {
		return Inspection{}, fmt.Errorf("%w: backup exceeds %d bytes", ErrInvalidBundle, int64(MaxUploadBytes))
	}
	files := filepath.Join(stage, "files")
	if err := os.Mkdir(files, 0o700); err != nil {
		return Inspection{}, err
	}
	if _, err := readBundle(bundlePath, passphrase, files); err != nil {
		return Inspection{}, err
	}
	_ = os.Remove(bundlePath)
	metadata, cfg, err := validateStagedBundle(ctx, files, m.databasePath)
	if err != nil {
		return Inspection{}, err
	}
	// Keep the validated configuration with this installation's database path.
	if err := cfg.Save(filepath.Join(files, "config.validated.yaml")); err != nil {
		return Inspection{}, err
	}
	summary, err := summarize(ctx, filepath.Join(files, "database.sqlite"), cfg, current, version)
	if err != nil {
		return Inspection{}, err
	}
	inspection := Inspection{Token: token, ExpiresAt: time.Now().Add(stageLifetime).UTC(), Metadata: metadata, Summary: summary}
	manifest, err := json.Marshal(inspection)
	if err != nil {
		return Inspection{}, err
	}
	if err := os.WriteFile(filepath.Join(stage, "inspection.json"), manifest, 0o600); err != nil {
		return Inspection{}, err
	}
	keep = true
	return inspection, nil
}

func cleanupStages(root string, now time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil && now.Sub(info.ModTime()) > stageLifetime {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

func summarize(ctx context.Context, path string, restored, current config.Config, version string) (Summary, error) {
	connection, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = connection.Close() }()
	summary := Summary{DNSListen: restored.DNS.Listen, Upstreams: restored.DNS.Upstreams, HTTPListen: restored.HTTP.Listen, CurrentVersion: version, Warnings: []string{}}
	counts := map[string]*int{"zones": &summary.Zones, "zone_records": &summary.Records, "blocklist_sources": &summary.Blocklists, "clients": &summary.Clients, "dns_rewrites": &summary.Rewrites, "forward_rules": &summary.ForwardRules, "users": &summary.Users}
	for table, target := range counts {
		// Older backups may predate a table; that simply counts as zero.
		var exists int
		if err := connection.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&exists); err != nil {
			return Summary{}, err
		}
		if exists == 0 {
			continue
		}
		if err := connection.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(target); err != nil {
			return Summary{}, err
		}
	}
	latest, err := database.LatestSchemaVersion()
	if err != nil {
		return Summary{}, err
	}
	summary.CurrentSchema = latest
	var backupSchema int
	if err := connection.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&backupSchema); err == nil && backupSchema < latest {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("The backup uses database schema %d; it will be upgraded to schema %d when Velora starts.", backupSchema, latest))
	}
	if summary.Users == 0 {
		summary.Warnings = append(summary.Warnings, "The backup contains no management users; you will need bootstrap credentials to sign in afterwards.")
	}
	if strings.Join(restored.DNS.Listen, ",") != strings.Join(current.DNS.Listen, ",") {
		summary.Warnings = append(summary.Warnings, "DNS listen addresses differ from the running configuration: "+strings.Join(restored.DNS.Listen, ", ")+".")
	}
	if restored.HTTP.Listen != current.HTTP.Listen {
		summary.Warnings = append(summary.Warnings, "The web interface will listen on "+restored.HTTP.Listen+" after the restore.")
	}
	sort.Strings(summary.Warnings)
	return summary, nil
}

func readState(databasePath string) (*RestoreState, error) {
	data, err := os.ReadFile(filepath.Join(dataDir(databasePath), resultFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state RestoreState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func writeState(databasePath string, state RestoreState) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(dataDir(databasePath), resultFile)
	temp, err := os.CreateTemp(dataDir(databasePath), ".velora-restore-result-*")
	if err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	return os.Rename(temp.Name(), target)
}

// RestoreStatus reports the last online restore, if any.
func (m *Manager) RestoreStatus() (*RestoreState, error) {
	if m.databasePath == "" {
		return nil, nil
	}
	return readState(m.databasePath)
}

// StageRestore turns an inspected backup into the pending restore that the
// next start applies. Any earlier pending restore is replaced.
func (m *Manager) StageRestore(token string) (Metadata, error) {
	if !tokenPattern.MatchString(token) {
		return Metadata{}, ErrStageNotFound
	}
	stage := filepath.Join(dataDir(m.databasePath), stagingDir, token)
	manifest, err := os.ReadFile(filepath.Join(stage, "inspection.json"))
	if err != nil {
		return Metadata{}, ErrStageNotFound
	}
	var inspection Inspection
	if err := json.Unmarshal(manifest, &inspection); err != nil || time.Now().After(inspection.ExpiresAt) {
		_ = os.RemoveAll(stage)
		return Metadata{}, ErrStageNotFound
	}
	pending := filepath.Join(dataDir(m.databasePath), pendingDir)
	if err := os.RemoveAll(pending); err != nil {
		return Metadata{}, err
	}
	if err := os.Rename(filepath.Join(stage, "files"), pending); err != nil {
		return Metadata{}, err
	}
	if err := os.WriteFile(filepath.Join(pending, "metadata.pending.json"), manifest, 0o600); err != nil {
		_ = os.RemoveAll(pending)
		return Metadata{}, err
	}
	_ = os.RemoveAll(stage)
	if err := writeState(m.databasePath, RestoreState{State: "pending", Metadata: inspection.Metadata}); err != nil {
		_ = os.RemoveAll(pending)
		return Metadata{}, err
	}
	return inspection.Metadata, nil
}

// PrepareStartup runs before the server opens its database. It applies a
// pending restore, or rolls back a restore whose previous start never
// became ready. It returns true when the configuration file changed.
func PrepareStartup(ctx context.Context, configPath, databasePath string) (bool, error) {
	if configPath == "" || databasePath == "" {
		return false, nil
	}
	state, err := readState(databasePath)
	if err != nil {
		return false, fmt.Errorf("read restore state: %w", err)
	}
	pending := filepath.Join(dataDir(databasePath), pendingDir)
	if state != nil && state.State == "applied" {
		// The last start used restored data and exited before it was ready.
		if err := rollbackTo(state.SafetyCopy, configPath, databasePath); err != nil {
			state.State, state.Error = "failed", "Velora did not become ready with the restored backup and the safety copy could not be restored: "+err.Error()
			_ = writeState(databasePath, *state)
			return false, errors.New(state.Error)
		}
		state.State, state.Error = "rolled_back", "Velora did not become ready with the restored backup; the previous data was restored."
		return true, writeState(databasePath, *state)
	}
	if _, err := os.Stat(filepath.Join(pending, "database.sqlite")); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	var metadata Metadata
	if manifest, err := os.ReadFile(filepath.Join(pending, "metadata.pending.json")); err == nil {
		var inspection Inspection
		if json.Unmarshal(manifest, &inspection) == nil {
			metadata = inspection.Metadata
		}
	}
	defer func() { _ = os.RemoveAll(pending) }()
	cfgBytes, err := os.ReadFile(filepath.Join(pending, "config.validated.yaml"))
	if err != nil {
		return false, err
	}
	var cfg config.Config
	if err := yaml.Unmarshal(cfgBytes, &cfg); err != nil {
		return false, err
	}
	safety, err := swapIn(ctx, filepath.Join(pending, "database.sqlite"), cfg, configPath, databasePath)
	if err != nil {
		_ = writeState(databasePath, RestoreState{State: "rolled_back", Metadata: metadata, SafetyCopy: safety, Error: "The restore could not be applied: " + err.Error()})
		return false, nil
	}
	return true, writeState(databasePath, RestoreState{State: "applied", Metadata: metadata, SafetyCopy: safety})
}

// ConfirmStartup marks an applied restore completed once the server is ready.
func ConfirmStartup(databasePath string) error {
	state, err := readState(databasePath)
	if err != nil || state == nil || state.State != "applied" {
		return err
	}
	state.State = "completed"
	return writeState(databasePath, *state)
}

// swapIn replaces the database and configuration with staged files,
// keeping a safety copy of the previous ones, and rolls back on failure.
func swapIn(ctx context.Context, stagedDatabase string, cfg config.Config, configPath, databasePath string) (string, error) {
	for _, path := range []string{configPath, databasePath} {
		info, err := os.Lstat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
			return "", fmt.Errorf("restore target is not a regular file: %s", path)
		}
	}
	lock, err := AcquireInstanceLock(databasePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = lock.Close() }()
	configStage := configPath + ".restore"
	_ = os.Remove(configStage)
	defer func() { _ = os.Remove(configStage) }()
	if err := cfg.Save(configStage); err != nil {
		return "", err
	}
	if _, err := os.Stat(databasePath); err == nil {
		if err := checkpointSQLite(ctx, databasePath); err != nil {
			return "", err
		}
	}
	safety := filepath.Join(dataDir(databasePath), fmt.Sprintf("velora-restore-safety-%s", time.Now().UTC().Format("20060102T150405.000000000")))
	if err := os.Mkdir(safety, 0o700); err != nil {
		return "", err
	}
	if _, err := os.Stat(databasePath); err == nil {
		if err := copyPrivate(databasePath, filepath.Join(safety, "database.sqlite")); err != nil {
			return safety, err
		}
	}
	if _, err := os.Stat(configPath); err == nil {
		if err := copyPrivate(configPath, filepath.Join(safety, "config.yaml")); err != nil {
			return safety, err
		}
	}
	fail := func(cause error) (string, error) {
		return safety, errors.Join(cause, rollbackTo(safety, configPath, databasePath))
	}
	_ = os.Remove(databasePath + "-wal")
	_ = os.Remove(databasePath + "-shm")
	if err := os.Rename(stagedDatabase, databasePath); err != nil {
		return fail(err)
	}
	if err := os.Rename(configStage, configPath); err != nil {
		return fail(err)
	}
	if err := checkApplicationState(ctx, databasePath, cfg); err != nil {
		return fail(err)
	}
	return safety, nil
}

func rollbackTo(safety, configPath, databasePath string) error {
	if safety == "" {
		return errors.New("no safety copy recorded")
	}
	var errs error
	for _, item := range []struct{ saved, target string }{{"database.sqlite", databasePath}, {"config.yaml", configPath}} {
		source := filepath.Join(safety, item.saved)
		if _, err := os.Stat(source); err != nil {
			continue
		}
		temp := item.target + ".rollback"
		_ = os.Remove(temp)
		if err := copyPrivate(source, temp); err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if item.saved == "database.sqlite" {
			_ = os.Remove(databasePath + "-wal")
			_ = os.Remove(databasePath + "-shm")
		}
		errs = errors.Join(errs, os.Rename(temp, item.target))
	}
	return errs
}
