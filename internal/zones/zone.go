// Package zones owns authoritative local data and its immutable resolver snapshots.
package zones

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("zone or record not found")
	ErrConflict = errors.New("zone changed")
	ErrExists   = errors.New("zone name already exists")
	ErrInvalid  = errors.New("invalid zone")
)

const (
	MaxZones        = 256
	MaxRecords      = 1000
	MaxTotalRecords = 10000
)

type Record struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      uint32 `json:"ttl"`
	Value    string `json:"value"`
	Priority uint16 `json:"priority"`
}
type Zone struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	PrimaryNS string   `json:"primary_ns"`
	Contact   string   `json:"contact"`
	Revision  uint32   `json:"revision"`
	Records   []Record `json:"records"`
}

// Repository persists whole-zone revisions atomically. Save may only assign IDs;
// all record content is normalized and validated by Service before persistence.
type Repository interface {
	LoadZones(context.Context) ([]Zone, error)
	SaveZone(context.Context, Zone, uint32) (Zone, error)
	DeleteZone(context.Context, int64, uint32) error
}

func clone(z Zone) Zone { z.Records = append([]Record{}, z.Records...); return z }
