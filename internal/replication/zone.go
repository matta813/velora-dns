// Package replication implements zone replication between nodes.
package replication

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/zones"
)

// ZoneReplicator manages zone replication between nodes.
type ZoneReplicator struct {
	zones    zones.Repository
	mu       sync.RWMutex
	logger   Logger
	replicas map[int64]*ZoneReplica
}

// ZoneReplica tracks replication state for a zone.
type ZoneReplica struct {
	ZoneID         int64
	PrimaryNode    string
	LastSyncAt     time.Time
	LastSyncSerial uint32
	Status         string
}

// NewZoneReplicator creates a new zone replicator.
func NewZoneReplicator(zones zones.Repository, logger Logger) *ZoneReplicator {
	return &ZoneReplicator{
		zones:    zones,
		logger:   logger,
		replicas: make(map[int64]*ZoneReplica),
	}
}

// Start begins zone replication monitoring.
func (r *ZoneReplicator) Start(ctx context.Context) {
	go r.monitorLoop(ctx)
}

func (r *ZoneReplicator) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.checkReplication(ctx)
		}
	}
}

func (r *ZoneReplicator) checkReplication(ctx context.Context) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, replica := range r.replicas {
		if replica.Status == "syncing" {
			continue
		}
		if time.Since(replica.LastSyncAt) > 5*time.Minute {
			r.logger.Info("zone replication stale", "zone_id", replica.ZoneID, "last_sync", replica.LastSyncAt)
		}
	}
}

// ReplicateZone replicates a zone from a primary node.
func (r *ZoneReplicator) ReplicateZone(ctx context.Context, zoneID int64, primaryNode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	replica := &ZoneReplica{
		ZoneID:      zoneID,
		PrimaryNode: primaryNode,
		Status:      "syncing",
		LastSyncAt:  time.Now(),
	}
	r.replicas[zoneID] = replica

	z, err := r.zones.LoadZones(ctx)
	if err != nil {
		replica.Status = "error"
		return fmt.Errorf("load zones for replication: %w", err)
	}

	for _, zone := range z {
		if zone.ID == zoneID {
			replica.LastSyncSerial = zone.Revision
			replica.Status = "synced"
			r.logger.Info("zone replicated", "zone_id", zoneID, "serial", zone.Revision)
			break
		}
	}

	return nil
}

// GetReplicaStatus returns the replication status for a zone.
func (r *ZoneReplicator) GetReplicaStatus(zoneID int64) (*ZoneReplica, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	replica, ok := r.replicas[zoneID]
	return replica, ok
}

// ListReplicas returns all replication states.
func (r *ZoneReplicator) ListReplicas() []*ZoneReplica {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ZoneReplica, 0, len(r.replicas))
	for _, replica := range r.replicas {
		out = append(out, replica)
	}
	return out
}
