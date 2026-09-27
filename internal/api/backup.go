package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/backupmeta"
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

// RestoreStore is implemented by backup managers that can restore online.
type RestoreStore interface {
	StageUpload(context.Context, io.Reader, string) (backupmeta.Inspection, error)
	StageRestore(token string) (backupmeta.Metadata, error)
	RestoreStatus() (*backupmeta.RestoreState, error)
}

// backupUploadLimit bounds uploaded bundles (plus multipart overhead).
const backupUploadLimit = backupmeta.MaxUploadBytes + 1<<20

func registerBackup(mux *http.ServeMux, store BackupStore, notify func(database.SystemEventInput), restart func()) {
	if restorer, ok := store.(RestoreStore); ok {
		registerRestore(mux, restorer, notify, restart)
	}
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
				notify(database.SystemEventInput{Type: "backup.failed", Key: "backup_failed", Severity: "warning", Title: "Backup failed", Message: "An encrypted backup could not be created.", Link: "/backup", Visibility: "admin"})
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
			notify(database.SystemEventInput{Type: "backup.created", Key: "backup_created", Severity: "info", Title: "Backup created", Message: "An encrypted configuration and database backup was downloaded.", Link: "/backup", Visibility: "admin"})
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

func registerRestore(mux *http.ServeMux, store RestoreStore, notify func(database.SystemEventInput), restart func()) {
	var restoring sync.Mutex
	mux.HandleFunc("POST /api/v1/backup/inspect", func(w http.ResponseWriter, r *http.Request) {
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now().Add(10 * time.Minute))
		_ = controller.SetWriteDeadline(time.Now().Add(12 * time.Minute))
		reader, err := r.MultipartReader()
		if err != nil {
			failure(w, 415, "unsupported_media_type", "Upload the backup as multipart/form-data with passphrase and bundle fields")
			return
		}
		// The passphrase part must come first so the bundle can be streamed.
		part, err := reader.NextPart()
		if err != nil || part.FormName() != "passphrase" {
			failure(w, 400, "invalid_upload", "Send the passphrase field before the bundle file")
			return
		}
		passphrase, err := io.ReadAll(io.LimitReader(part, 1024))
		if err != nil || len(passphrase) < 12 {
			failure(w, 400, "invalid_passphrase", "Passphrase must contain at least 12 characters")
			return
		}
		part, err = reader.NextPart()
		if err != nil || part.FormName() != "bundle" {
			failure(w, 400, "invalid_upload", "The bundle file is missing")
			return
		}
		inspection, err := store.StageUpload(r.Context(), part, string(passphrase))
		if err != nil {
			var tooLarge *http.MaxBytesError
			switch {
			case errors.As(err, &tooLarge):
				failure(w, 413, "payload_too_large", "The backup is too large")
			case errors.Is(err, backupmeta.ErrInvalidBundle):
				failure(w, 400, "invalid_backup", "The file is not a valid Velora backup, or the passphrase is wrong")
			default:
				failure(w, 400, "invalid_backup", "The backup cannot be restored here: "+err.Error())
			}
			return
		}
		respond(w, 200, inspection)
	})
	mux.HandleFunc("GET /api/v1/backup/restore", func(w http.ResponseWriter, r *http.Request) {
		state, err := store.RestoreStatus()
		if err != nil {
			failure(w, 503, "restore_status_unavailable", "Restore status is unavailable")
			return
		}
		respond(w, 200, state)
	})
	mux.HandleFunc("POST /api/v1/backup/restore", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Token   string `json:"token"`
			Confirm bool   `json:"confirm"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if !input.Confirm {
			failure(w, 400, "confirmation_required", "Confirm that the current configuration and data will be replaced")
			return
		}
		if !restoring.TryLock() {
			failure(w, 409, "restore_in_progress", "A restore is already being scheduled")
			return
		}
		metadata, err := store.StageRestore(input.Token)
		if err != nil {
			restoring.Unlock()
			if errors.Is(err, backupmeta.ErrStageNotFound) {
				failure(w, 404, "backup_not_found", err.Error())
				return
			}
			failure(w, 503, "restore_failed", "The restore could not be scheduled")
			return
		}
		if notify != nil {
			notify(database.SystemEventInput{Key: "backup_restore", Severity: "warning", Title: "Restore scheduled", Message: "A backup from " + metadata.CreatedAt.Format(time.RFC3339) + " will replace the configuration and database on restart.", Link: "/backup", Visibility: "admin"})
		}
		state := "restarting"
		if restart == nil {
			state = "pending"
			restoring.Unlock()
		}
		respond(w, 202, map[string]any{"state": state, "metadata": metadata})
		if restart != nil {
			// Let the response reach the browser before the server stops.
			go func() {
				time.Sleep(500 * time.Millisecond)
				restart()
			}()
		}
	})
}
