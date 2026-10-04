package api

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/backupmeta"
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

type fakeRestore struct {
	fakeEncryptedBackup
	uploaded   []byte
	passphrase string
	staged     string
	state      *backupmeta.RestoreState
}

func (f *fakeRestore) StageUpload(_ context.Context, upload io.Reader, passphrase string) (backupmeta.Inspection, error) {
	data, err := io.ReadAll(upload)
	if err != nil {
		return backupmeta.Inspection{}, err
	}
	if string(data) != "VELBK001 bundle" {
		return backupmeta.Inspection{}, backupmeta.ErrInvalidBundle
	}
	f.uploaded, f.passphrase = data, passphrase
	return backupmeta.Inspection{Token: "0123456789abcdef0123456789abcdef", Metadata: backupmeta.Metadata{FormatVersion: 1, VeloraVersion: "1.0.0"}}, nil
}
func (f *fakeRestore) StageRestore(token string) (backupmeta.Metadata, error) {
	if token != "0123456789abcdef0123456789abcdef" {
		return backupmeta.Metadata{}, backupmeta.ErrStageNotFound
	}
	f.staged = token
	f.state = &backupmeta.RestoreState{State: "pending"}
	return backupmeta.Metadata{FormatVersion: 1}, nil
}
func (f *fakeRestore) RestoreStatus() (*backupmeta.RestoreState, error) { return f.state, nil }

func multipartUpload(t *testing.T, fields [][2]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, field := range fields {
		if field[0] == "bundle" {
			part, err := writer.CreateFormFile("bundle", "velora-backup.vdns")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = part.Write([]byte(field[1]))
			continue
		}
		_ = writer.WriteField(field[0], field[1])
	}
	_ = writer.Close()
	return body, writer.FormDataContentType()
}

func TestOnlineRestoreEndpoints(t *testing.T) {
	db, err := database.Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, user := range []struct{ name, role string }{{"admin", "admin"}, {"operator", "operator"}} {
		if _, err := db.CreateUser(context.Background(), user.name, "a sufficiently long password", user.role); err != nil {
			t.Fatal(err)
		}
	}
	store := &fakeRestore{}
	restarts := make(chan struct{}, 1)
	memory := cache.New(10)
	handler := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: memory, Metrics: metrics.New(memory), Config: config.Default(), Started: time.Now(), Backup: store, RequestRestart: func() { restarts <- struct{}{} }})
	adminCookie, csrf := loginForTest(t, handler, "admin", "a sufficiently long password")
	operatorCookie, operatorCSRF := loginForTest(t, handler, "operator", "a sufficiently long password")
	upload := func(cookie *http.Cookie, token string, fields [][2]string) *httptest.ResponseRecorder {
		body, contentType := multipartUpload(t, fields)
		r := httptest.NewRequest("POST", "http://127.0.0.1/api/v1/backup/inspect", body)
		r.Header.Set("Content-Type", contentType)
		r.Header.Set("X-CSRF-Token", token)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	good := [][2]string{{"passphrase", "a sufficiently long passphrase"}, {"bundle", "VELBK001 bundle"}}
	if w := upload(operatorCookie, operatorCSRF, good); w.Code != 403 {
		t.Fatalf("operator inspect: %d", w.Code)
	}
	if w := upload(adminCookie, "", good); w.Code != 403 {
		t.Fatalf("inspect without CSRF: %d", w.Code)
	}
	if w := upload(adminCookie, csrf, [][2]string{{"bundle", "VELBK001 bundle"}, {"passphrase", "a sufficiently long passphrase"}}); w.Code != 400 {
		t.Fatalf("passphrase must come first: %d %s", w.Code, w.Body.String())
	}
	if w := upload(adminCookie, csrf, [][2]string{{"passphrase", "short"}, {"bundle", "VELBK001 bundle"}}); w.Code != 400 {
		t.Fatalf("short passphrase: %d", w.Code)
	}
	if w := upload(adminCookie, csrf, [][2]string{{"passphrase", "a sufficiently long passphrase"}, {"bundle", "garbage"}}); w.Code != 400 || !strings.Contains(w.Body.String(), "not a valid Velora backup") {
		t.Fatalf("invalid bundle: %d %s", w.Code, w.Body.String())
	}
	w := upload(adminCookie, csrf, good)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"token":"0123456789abcdef0123456789abcdef"`) || store.passphrase != "a sufficiently long passphrase" {
		t.Fatalf("inspect: %d %s", w.Code, w.Body.String())
	}
	restore := `{"token":"0123456789abcdef0123456789abcdef","confirm":true}`
	if w = authRequest(handler, "POST", "/api/v1/backup/restore", restore, operatorCookie, operatorCSRF); w.Code != 403 {
		t.Fatalf("operator restore: %d", w.Code)
	}
	if w = authRequest(handler, "POST", "/api/v1/backup/restore", `{"token":"0123456789abcdef0123456789abcdef","confirm":false}`, adminCookie, csrf); w.Code != 400 {
		t.Fatalf("unconfirmed: %d", w.Code)
	}
	if w = authRequest(handler, "POST", "/api/v1/backup/restore", `{"token":"ffffffffffffffffffffffffffffffff","confirm":true}`, adminCookie, csrf); w.Code != 404 {
		t.Fatalf("unknown token: %d", w.Code)
	}
	if w = authRequest(handler, "POST", "/api/v1/backup/restore", restore, adminCookie, csrf); w.Code != 202 || !strings.Contains(w.Body.String(), `"state":"restarting"`) {
		t.Fatalf("restore: %d %s", w.Code, w.Body.String())
	}
	select {
	case <-restarts:
	case <-time.After(3 * time.Second):
		t.Fatal("restart was not requested")
	}
	if w = authRequest(handler, "POST", "/api/v1/backup/restore", restore, adminCookie, csrf); w.Code != 409 {
		t.Fatalf("second restore while restarting: %d", w.Code)
	}
	if w = authRequest(handler, "GET", "/api/v1/backup/restore", "", adminCookie, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"pending"`) {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
}
