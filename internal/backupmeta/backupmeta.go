// Package backupmeta holds the backup types shared by the backup
// implementation and the management API.
package backupmeta

import (
	"errors"
	"time"
)

// MaxUploadBytes bounds a bundle uploaded for online restore.
const MaxUploadBytes = 2 << 30

var (
	ErrInvalidBundle = errors.New("invalid or incompatible backup bundle")
	ErrStageNotFound = errors.New("inspected backup not found or expired; inspect it again")
)

type Metadata struct {
	FormatVersion int       `json:"format_version"`
	VeloraVersion string    `json:"velora_version"`
	CreatedAt     time.Time `json:"created_at"`
	SchemaVersion int       `json:"schema_version"`
	Components    []string  `json:"components"`
}

// Summary describes what a staged backup would restore.
type Summary struct {
	Zones          int      `json:"zones"`
	Records        int      `json:"records"`
	Blocklists     int      `json:"blocklists"`
	Clients        int      `json:"clients"`
	Rewrites       int      `json:"rewrites"`
	ForwardRules   int      `json:"forward_rules"`
	Users          int      `json:"users"`
	DNSListen      []string `json:"dns_listen"`
	Upstreams      []string `json:"upstreams"`
	HTTPListen     string   `json:"http_listen"`
	CurrentVersion string   `json:"current_version"`
	CurrentSchema  int      `json:"current_schema"`
	Warnings       []string `json:"warnings"`
}

// Inspection is returned after a successful upload.
type Inspection struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Metadata  Metadata  `json:"metadata"`
	Summary   Summary   `json:"summary"`
}

// RestoreState is persisted next to the database so the result survives
// the restart that applies it.
type RestoreState struct {
	State      string    `json:"state"` // pending, applied, completed, rolled_back, failed
	Metadata   Metadata  `json:"metadata"`
	SafetyCopy string    `json:"safety_copy,omitempty"`
	Error      string    `json:"error,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}
