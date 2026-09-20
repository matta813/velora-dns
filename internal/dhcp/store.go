package dhcp

import (
	"context"
	"net/netip"
)

// Store defines the persistence contract for DHCP leases, pools, and reservations.
type Store interface {
	// Pool operations
	CreatePool(ctx context.Context, pool *Pool) error
	GetPool(ctx context.Context, id int64) (*Pool, error)
	ListPools(ctx context.Context) ([]Pool, error)
	UpdatePool(ctx context.Context, pool *Pool) error
	DeletePool(ctx context.Context, id int64) error

	// Reservation operations
	CreateReservation(ctx context.Context, r *Reservation) error
	ListReservations(ctx context.Context, poolID int64) ([]Reservation, error)
	DeleteReservation(ctx context.Context, id int64) error

	// Lease operations
	SaveLease(ctx context.Context, l *Lease) error
	GetLease(ctx context.Context, poolID int64, mac string) (*Lease, error)
	GetLeaseByIP(ctx context.Context, poolID int64, ip netip.Addr) (*Lease, error)
	ListLeases(ctx context.Context, poolID int64) ([]Lease, error)
	ListActiveLeases(ctx context.Context) ([]Lease, error)
	ExpireLeases(ctx context.Context) (int64, error)
	DeleteLease(ctx context.Context, id int64) error
}
