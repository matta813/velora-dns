package database

import (
	"context"
	"database/sql"
	"sync"
)

// Cluster manages a primary PostgreSQL connection and zero or more read replicas.
type Cluster struct {
	primary  *sql.DB
	replicas []*sql.DB
	mu       sync.RWMutex
}

// NewCluster creates a new cluster with a primary and optional replicas.
func NewCluster(primary *sql.DB, replicas ...*sql.DB) *Cluster {
	return &Cluster{
		primary:  primary,
		replicas: replicas,
	}
}

// Primary returns the primary (read/write) connection.
func (c *Cluster) Primary() *sql.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.primary
}

// Replica returns a read replica if available, otherwise the primary.
func (c *Cluster) Replica() *sql.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.replicas) > 0 {
		return c.replicas[0]
	}
	return c.primary
}

// HealthCheck verifies connectivity to the primary.
func (c *Cluster) HealthCheck(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.primary != nil {
		return c.primary.PingContext(ctx)
	}
	return nil
}
