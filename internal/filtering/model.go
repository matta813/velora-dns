package filtering

import (
	"context"
	"errors"
	"time"
)

const MaxSources = 32
const MaxDomains = 100000
const MaxTotalDomains = 250000

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
}
type SourceStore interface {
	LoadSources(context.Context) ([]Source, error)
	SaveSource(context.Context, Source) (Source, error)
	DeleteSource(context.Context, int64) error
}
