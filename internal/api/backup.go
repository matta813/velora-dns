package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/matta813/velora-dns/internal/database"
)

type BackupStore interface {
	BackupStatus() (*BackupStatus, error)
	VerifyBackup(path string) (*BackupVerification, error)
	CreateEncryptedBackup(context.Context, string) (string, error)
}

type BackupStatus struct {
	Supported         bool       `json:"supported"`
	LastBackupTime    *time.Time `json:"last_backup_time,omitempty"`
	LastBackupSize    int64      `json:"last_backup_size,omitempty"`
	BackupAge         string     `json:"backup_age,omitempty"`
	VerificationState string     `json:"verification_state"`
	DatabasePath      string     `json:"database_path"`
}

type BackupVerification struct {
	Valid         bool   `json:"valid"`
	SchemaVersion int    `json:"schema_version"`
	RecordCount   int    `json:"record_count"`
	Error         string `json:"error,omitempty"`
}

func registerBackup(mux *http.ServeMux, store BackupStore, notify func(database.SystemEventInput)) {
	mux.HandleFunc("POST /api/v1/backup/create", func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			failure(w, 503, "backup_unavailable", "Backup download is unavailable")
			return
		}
		var input struct {
			Passphrase string `json:"passphrase"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if len(input.Passphrase) < 12 {
			failure(w, 400, "invalid_passphrase", "Passphrase must contain at least 12 characters")
			return
		}
		path, err := store.CreateEncryptedBackup(r.Context(), input.Passphrase)
		if err != nil {
			if notify != nil {
				notify(database.SystemEventInput{Key: "backup_failed", Severity: "warning", Title: "Backup failed", Message: "An encrypted backup could not be created.", Link: "/backup", Visibility: "admin"})
			}
			failure(w, 503, "backup_failed", "Could not create encrypted backup")
			return
		}
		defer func() { _ = os.Remove(path) }()
		file, err := os.Open(path)
		if err != nil {
			failure(w, 503, "backup_failed", "Could not open encrypted backup")
			return
		}
		defer func() { _ = file.Close() }()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=velora-backup.vdns")
		w.Header().Set("Cache-Control", "no-store")
		_, copyErr := io.Copy(w, file)
		if notify != nil && copyErr == nil {
			notify(database.SystemEventInput{Key: "backup_created", Severity: "info", Title: "Backup created", Message: "An encrypted configuration and database backup was downloaded.", Link: "/backup", Visibility: "admin"})
		}
	})
	mux.HandleFunc("GET /api/v1/backup/status", func(w http.ResponseWriter, r *http.Request) {
		status, err := store.BackupStatus()
		if err != nil {
			failure(w, 503, "backup_error", "Could not retrieve backup status")
			return
		}
		respond(w, 200, status)
	})

	mux.HandleFunc("POST /api/v1/backup/verify", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Path string `json:"path"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if input.Path == "" {
			failure(w, 400, "missing_path", "Backup path is required")
			return
		}

		user := r.Context().Value(authContextKey{})
		if user == nil {
			failure(w, 401, "authentication_required", "Authentication required")
			return
		}

		verification, err := store.VerifyBackup(input.Path)
		if err != nil {
			failure(w, 500, "verification_error", "Backup verification failed")
			return
		}
		respond(w, 200, verification)
	})
}
