package api

import (
	"net/http"
	"time"
)

type BackupStore interface {
	BackupStatus() (*BackupStatus, error)
	VerifyBackup(path string) (*BackupVerification, error)
}

type BackupStatus struct {
	LastBackupTime    time.Time `json:"last_backup_time,omitempty"`
	LastBackupSize    int64     `json:"last_backup_size,omitempty"`
	BackupAge         string    `json:"backup_age,omitempty"`
	VerificationState string    `json:"verification_state"`
	DatabasePath      string    `json:"database_path"`
}

type BackupVerification struct {
	Valid         bool   `json:"valid"`
	SchemaVersion int    `json:"schema_version"`
	RecordCount   int    `json:"record_count"`
	Error         string `json:"error,omitempty"`
}

func registerBackup(mux *http.ServeMux, store BackupStore) {
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
