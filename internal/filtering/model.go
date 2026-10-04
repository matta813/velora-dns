package filtering

import (
	"context"
	"errors"
	"time"
)

const MaxSources = 32
const MaxDomains = 100000
const MaxTotalDomains = 250000

// Scheduled refresh bounds. Zero disables scheduling (manual refresh only).
const (
	MinUpdateInterval = 3600
	MaxUpdateInterval = 7 * 24 * 3600
)

var ErrInvalid = errors.New("invalid blocklist")
var ErrNotFound = errors.New("blocklist not found")
var ErrExists = errors.New("blocklist name exists")
var ErrBusy = errors.New("blocklist operation already in progress")

type Source struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Enabled       bool       `json:"enabled"`
	LastUpdatedAt *time.Time `json:"last_updated_at"`
	LastError     string     `json:"last_error"`
	DomainCount   int        `json:"domain_count"`
	Domains       []string   `json:"-"`
	// UpdateInterval is the scheduled refresh period in seconds; zero means
	// the list is only refreshed manually.
	UpdateInterval      int        `json:"update_interval"`
	LastAttemptAt       *time.Time `json:"last_attempt_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	// NextUpdateAt is derived, never stored: see Service.nextUpdate.
	NextUpdateAt *time.Time `json:"next_update_at"`
}

// ValidInterval reports whether seconds is an accepted refresh schedule.
func ValidInterval(seconds int) bool {
	return seconds == 0 || (seconds >= MinUpdateInterval && seconds <= MaxUpdateInterval)
}

type SourceStore interface {
	LoadSources(context.Context) ([]Source, error)
	SaveSource(context.Context, Source) (Source, error)
	DeleteSource(context.Context, int64) error
}
