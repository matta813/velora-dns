package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/metrics"
)

type fakeEncryptedBackup struct{ path string }

func (f fakeEncryptedBackup) BackupStatus() (*BackupStatus, error) { return &BackupStatus{}, nil }
func (f fakeEncryptedBackup) VerifyBackup(string) (*BackupVerification, error) {
	return &BackupVerification{}, nil
}
func (f fakeEncryptedBackup) CreateEncryptedBackup(context.Context, string) (string, error) {
	return f.path, nil
}

func TestEncryptedBackupRequiresAdminAndCSRF(t *testing.T) {
	db, err := database.Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, user := range []struct{ name, role string }{{"admin", "admin"}, {"viewer", "viewer"}} {
		if _, err := db.CreateUser(context.Background(), user.name, "a sufficiently long password", user.role); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "backup.vdns")
	if err := os.WriteFile(path, []byte("encrypted archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	memory := cache.New(10)
	handler := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: memory, Metrics: metrics.New(memory), Config: config.Default(), Started: time.Now(), Backup: fakeEncryptedBackup{path}})
	adminCookie, csrf := loginForTest(t, handler, "admin", "a sufficiently long password")
	viewerCookie, viewerCSRF := loginForTest(t, handler, "viewer", "a sufficiently long password")
	body := `{"passphrase":"a sufficiently long passphrase"}`
	if w := authRequest(handler, "POST", "/api/v1/backup/create", body, adminCookie, ""); w.Code != 403 {
		t.Fatalf("missing CSRF: %d", w.Code)
	}
	if w := authRequest(handler, "POST", "/api/v1/backup/create", body, viewerCookie, viewerCSRF); w.Code != 403 {
		t.Fatalf("viewer backup: %d", w.Code)
	}
	if w := authRequest(handler, "POST", "/api/v1/backup/create", body, adminCookie, csrf); w.Code != 200 || w.Body.String() != "encrypted archive" {
		t.Fatalf("admin backup: %d %q", w.Code, w.Body.String())
	}
}
