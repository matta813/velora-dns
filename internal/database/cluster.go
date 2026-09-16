// Package database implements PostgreSQL cluster support.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// ClusterConfig holds PostgreSQL cluster configuration.
type ClusterConfig struct {
	PrimaryDSN      string
	ReplicaDSNs     []string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// Cluster manages a PostgreSQL primary/replica setup.
type Cluster struct {
	primary  *sql.DB
	replicas []*sql.DB
	config   ClusterConfig
	mu       sync.RWMutex
	logger   ClusterLogger
}

// ClusterLogger is a logger for cluster operations.
type ClusterLogger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
}

// NewCluster creates a new PostgreSQL cluster connection.
func NewCluster(config ClusterConfig, logger ClusterLogger) (*Cluster, error) {
	primary, err := sql.Open("pgx", config.PrimaryDSN)
	if err != nil {
		return nil, fmt.Errorf("open primary: %w", err)
	}

	primary.SetMaxOpenConns(config.MaxOpenConns)
	primary.SetMaxIdleConns(config.MaxIdleConns)
	primary.SetConnMaxLifetime(config.ConnMaxLifetime)

	if err := primary.Ping(); err != nil {
		primary.Close()
		return nil, fmt.Errorf("ping primary: %w", err)
	}

	c := &Cluster{
		primary: primary,
		config:  config,
		logger:  logger,
	}

	for _, dsn := range config.ReplicaDSNs {
		replica, err := sql.Open("pgx", dsn)
		if err != nil {
			logger.Warn("failed to connect replica", "dsn", maskDSN(dsn), "error", err)
			continue
		}
		replica.SetMaxOpenConns(config.MaxOpenConns)
		replica.SetMaxIdleConns(config.MaxIdleConns)
		replica.SetConnMaxLifetime(config.ConnMaxLifetime)
		c.replicas = append(c.replicas, replica)
	}

	return c, nil
}

// Primary returns the primary database connection.
func (c *Cluster) Primary() *sql.DB {
	return c.primary
}

// Replica returns a read replica using round-robin.
func (c *Cluster) Replica() *sql.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.replicas) == 0 {
		return c.primary
	}
	// Simple round-robin based on time
	idx := int(time.Now().UnixNano()) % len(c.replicas)
	return c.replicas[idx]
}

// Close closes all database connections.
func (c *Cluster) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error
	for _, replica := range c.replicas {
		if err := replica.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := c.primary.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("close cluster: %v", errs)
	}
	return nil
}

// HealthCheck checks the health of all connections.
func (c *Cluster) HealthCheck(ctx context.Context) error {
	if err := c.primary.PingContext(ctx); err != nil {
		return fmt.Errorf("primary unhealthy: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for i, replica := range c.replicas {
		if err := replica.PingContext(ctx); err != nil {
			c.logger.Warn("replica unhealthy", "index", i, "error", err)
		}
	}
	return nil
}

func maskDSN(dsn string) string {
	if len(dsn) > 20 {
		return dsn[:20] + "***"
	}
	return "***"
}
