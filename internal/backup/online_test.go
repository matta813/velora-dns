package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
)

const testPassphrase = "a sufficiently long passphrase"

type installation struct {
	dir, dbPath, configPath string
	cfg                     config.Config
	db                      *database.Store
	manager                 *Manager
	bundle                  []byte
}

// newInstallation creates an installation with user "original", takes a
// bundle of it, then diverges (user "later", debug logging).
func newInstallation(t *testing.T) *installation {
	t.Helper()
	ctx := context.Background()
	in := &installation{dir: t.TempDir()}
	in.dbPath, in.configPath = filepath.Join(in.dir, "velora.db"), filepath.Join(in.dir, "config.yaml")
	in.cfg = config.Default()
	in.cfg.DatabasePath = in.dbPath
	if err := in.cfg.Save(in.configPath); err != nil {
		t.Fatal(err)
	}
	var err error
	if in.db, err = database.Open(ctx, "sqlite", in.dbPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.db.Close() })
	if _, err = in.db.CreateUser(ctx, "original", "long original password", "admin"); err != nil {
		t.Fatal(err)
	}
	in.manager = NewManager(in.db, in.dbPath)
	in.manager.SetConfig(in.cfg, "test-version")
	path, _, err := in.manager.CreateBundle(ctx, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if in.bundle, err = os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err = in.db.CreateUser(ctx, "later", "long second password", "viewer"); err != nil {
		t.Fatal(err)
	}
	in.cfg.LogLevel = "debug"
	in.manager.SetConfig(in.cfg, "test-version")
	if err = in.cfg.Save(in.configPath); err != nil {
		t.Fatal(err)
	}
	return in
}

func (in *installation) users(t *testing.T) int {
	t.Helper()
	db, err := database.Open(context.Background(), "sqlite", in.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	count, err := db.UserCount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestOnlineRestoreStagesAppliesAndConfirms(t *testing.T) {
	ctx := context.Background()
	in := newInstallation(t)
	inspection, err := in.manager.StageUpload(ctx, bytes.NewReader(in.bundle), testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Summary.Users != 1 || inspection.Metadata.VeloraVersion != "test-version" || len(inspection.Token) != 32 {
		t.Fatalf("inspection: %+v", inspection)
	}
	if in.users(t) != 2 {
		t.Fatal("inspection must not modify the installation")
	}
	if _, err = in.manager.StageRestore("0123456789abcdef0123456789abcdef"); !errors.Is(err, ErrStageNotFound) {
		t.Fatalf("unknown token: %v", err)
	}
	if _, err = in.manager.StageRestore("../../etc"); !errors.Is(err, ErrStageNotFound) {
		t.Fatalf("path token: %v", err)
	}
	if _, err = in.manager.StageRestore(inspection.Token); err != nil {
		t.Fatal(err)
	}
	if state, _ := in.manager.RestoreStatus(); state == nil || state.State != "pending" {
		t.Fatalf("pending state: %+v", state)
	}
	// The server stops (releasing the database), then starts again.
	if err = in.db.Close(); err != nil {
		t.Fatal(err)
	}
	changed, err := PrepareStartup(ctx, in.configPath, in.dbPath)
	if err != nil || !changed {
		t.Fatalf("prepare: %v %v", changed, err)
	}
	if in.users(t) != 1 {
		t.Fatal("restored database should contain only the original user")
	}
	restored, err := config.Load(in.configPath)
	if err != nil || restored.LogLevel != "info" || restored.DatabasePath != in.dbPath {
		t.Fatalf("restored config: %+v %v", restored.LogLevel, err)
	}
	state, _ := readState(in.dbPath)
	if state.State != "applied" || state.SafetyCopy == "" {
		t.Fatalf("applied: %+v", state)
	}
	if _, err = os.Stat(filepath.Join(state.SafetyCopy, "database.sqlite")); err != nil {
		t.Fatalf("safety copy: %v", err)
	}
	if err = ConfirmStartup(in.dbPath); err != nil {
		t.Fatal(err)
	}
	if state, _ = readState(in.dbPath); state.State != "completed" {
		t.Fatalf("completed: %+v", state)
	}
	// Later starts leave everything alone.
	if changed, err = PrepareStartup(ctx, in.configPath, in.dbPath); err != nil || changed {
		t.Fatalf("idle start: %v %v", changed, err)
	}
}

func TestRestoreRollsBackWhenRestartNeverBecomesReady(t *testing.T) {
	ctx := context.Background()
	in := newInstallation(t)
	inspection, err := in.manager.StageUpload(ctx, bytes.NewReader(in.bundle), testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = in.manager.StageRestore(inspection.Token); err != nil {
		t.Fatal(err)
	}
	_ = in.db.Close()
	if _, err = PrepareStartup(ctx, in.configPath, in.dbPath); err != nil {
		t.Fatal(err)
	}
	// No ConfirmStartup: the restored start crashed. The next start rolls back.
	changed, err := PrepareStartup(ctx, in.configPath, in.dbPath)
	if err != nil || !changed {
		t.Fatalf("rollback start: %v %v", changed, err)
	}
	state, _ := readState(in.dbPath)
	if state.State != "rolled_back" || !strings.Contains(state.Error, "did not become ready") {
		t.Fatalf("state: %+v", state)
	}
	if in.users(t) != 2 {
		t.Fatal("previous database was not restored")
	}
	if cfg, err := config.Load(in.configPath); err != nil || cfg.LogLevel != "debug" {
		t.Fatalf("previous config was not restored: %v", err)
	}
}

func TestStageUploadRejectsBadBundlesWithoutSideEffects(t *testing.T) {
	ctx := context.Background()
	in := newInstallation(t)
	if _, err := in.manager.StageUpload(ctx, bytes.NewReader(in.bundle), "wrong but long passphrase"); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	if _, err := in.manager.StageUpload(ctx, strings.NewReader("not a backup at all"), testPassphrase); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("garbage: %v", err)
	}
	truncated := in.bundle[:len(in.bundle)/2]
	if _, err := in.manager.StageUpload(ctx, bytes.NewReader(truncated), testPassphrase); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("truncated: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(in.dir, stagingDir))
	if len(entries) != 0 {
		t.Fatalf("failed uploads left %d staging directories", len(entries))
	}
	if state, _ := in.manager.RestoreStatus(); state != nil {
		t.Fatalf("no restore should be recorded: %+v", state)
	}
	if in.users(t) != 2 {
		t.Fatal("installation changed")
	}
}
