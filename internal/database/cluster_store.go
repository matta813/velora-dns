package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/node"
	"github.com/matta813/velora-dns/internal/replication"
	"github.com/matta813/velora-dns/internal/zones"
)

// ClusterStore extends Store with cluster-aware read/write routing.
type ClusterStore struct {
	cluster *Cluster
	*Store
}

// NewClusterStore creates a store backed by a PostgreSQL cluster.
func NewClusterStore(cluster *Cluster, driver string) *ClusterStore {
	return &ClusterStore{
		cluster: cluster,
		Store: &Store{
			db:    cluster.Primary(),
			driver: driver,
		},
	}
}

// ReadWrite returns a connection for read/write operations (primary).
func (cs *ClusterStore) ReadWrite() *sql.DB {
	return cs.cluster.Primary()
}

// ReadOnly returns a read replica if available, otherwise primary.
func (cs *ClusterStore) ReadOnly() *sql.DB {
	return cs.cluster.Replica()
}

// SaveZone writes zone to primary.
func (cs *ClusterStore) SaveZone(ctx context.Context, z zones.Zone, expected uint32) (zones.Zone, error) {
	return cs.Store.SaveZone(ctx, z, expected)
}

// LoadZones reads zones from replica.
func (cs *ClusterStore) LoadZones(ctx context.Context) ([]zones.Zone, error) {
	db := cs.cluster.Replica()
	return cs.loadZonesFromDB(ctx, db)
}

func (cs *ClusterStore) loadZonesFromDB(ctx context.Context, db *sql.DB) ([]zones.Zone, error) {
	p := cs.placeholder
	query := fmt.Sprintf("SELECT id,name,primary_ns,contact,revision,zone_type,primary_address,transfer_tsig_key,transfer_interval,last_transfer_at,next_refresh_at,last_transfer_serial FROM zones ORDER BY id LIMIT %s", p(1))
	rows, err := db.QueryContext(ctx, query, zones.MaxZones+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	all := []zones.Zone{}
	index := map[int64]int{}
	for rows.Next() {
		var z zones.Zone
		var zoneType, primaryAddress, transferTSIGKey string
		var transferInterval int
		var lastTransferAt, nextRefreshAt sql.NullString
		var lastTransferSerial int64
		if err = rows.Scan(&z.ID, &z.Name, &z.PrimaryNS, &z.Contact, &z.Revision, &zoneType, &primaryAddress, &transferTSIGKey, &transferInterval, &lastTransferAt, &nextRefreshAt, &lastTransferSerial); err != nil {
			return nil, err
		}
		z.ZoneType = zoneType
		z.PrimaryAddress = primaryAddress
		z.TransferTSIGKey = transferTSIGKey
		z.TransferInterval = transferInterval
		z.LastTransferSerial = uint32(lastTransferSerial)
		if lastTransferAt.Valid {
			t, err := time.Parse(time.RFC3339, lastTransferAt.String)
			if err == nil {
				z.LastTransferAt = &t
			}
		}
		if nextRefreshAt.Valid {
			t, err := time.Parse(time.RFC3339, nextRefreshAt.String)
			if err == nil {
				z.NextRefreshAt = &t
			}
		}
		z.Records = []zones.Record{}
		index[z.ID] = len(all)
		all = append(all, z)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(all) > zones.MaxZones {
		return nil, fmt.Errorf("stored zones exceed configured service limit")
	}

	query = fmt.Sprintf("SELECT id,zone_id,name,type,ttl,value,priority FROM zone_records ORDER BY id LIMIT %s", p(1))
	rows, err = db.QueryContext(ctx, query, zones.MaxTotalRecords+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		count++
		var r zones.Record
		var id int64
		if err = rows.Scan(&r.ID, &id, &r.Name, &r.Type, &r.TTL, &r.Value, &r.Priority); err != nil {
			return nil, err
		}
		i, ok := index[id]
		if !ok {
			return nil, fmt.Errorf("record references missing zone")
		}
		all[i].Records = append(all[i].Records, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if count > zones.MaxTotalRecords {
		return nil, fmt.Errorf("stored records exceed service limit")
	}
	return all, nil
}

// SaveNode writes node to primary.
func (cs *ClusterStore) SaveNode(ctx context.Context, n node.Node) error {
	return cs.Store.SaveNode(ctx, n)
}

// LoadNodes reads nodes from replica.
func (cs *ClusterStore) LoadNodes(ctx context.Context) ([]node.Node, error) {
	db := cs.cluster.Replica()
	return cs.loadNodesFromDB(ctx, db)
}

func (cs *ClusterStore) loadNodesFromDB(ctx context.Context, db *sql.DB) ([]node.Node, error) {
	rows, err := db.QueryContext(ctx, "SELECT id,name,address,capabilities,version,status,last_seen_at FROM nodes ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var nodes []node.Node
	for rows.Next() {
		var n node.Node
		if err = rows.Scan(&n.ID, &n.Name, &n.Address, &n.Capabilities, &n.Version, &n.Status, &n.LastSeenAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// SaveConfigVersion writes config version to primary.
func (cs *ClusterStore) SaveConfigVersion(ctx context.Context, v replication.ConfigVersion) error {
	return cs.Store.SaveConfigVersion(ctx, v)
}

// LoadConfigVersions reads config versions from replica.
func (cs *ClusterStore) LoadConfigVersions(ctx context.Context, limit int) ([]replication.ConfigVersion, error) {
	db := cs.cluster.Replica()
	return cs.loadConfigVersionsFromDB(ctx, db, limit)
}

func (cs *ClusterStore) loadConfigVersionsFromDB(ctx context.Context, db *sql.DB, limit int) ([]replication.ConfigVersion, error) {
	p := cs.placeholder
	query := fmt.Sprintf("SELECT version,config_hash,applied_by,applied_at FROM config_versions ORDER BY version DESC LIMIT %s", p(1))
	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var versions []replication.ConfigVersion
	for rows.Next() {
		var v replication.ConfigVersion
		if err = rows.Scan(&v.Version, &v.ConfigHash, &v.AppliedBy, &v.AppliedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// Migrate runs migrations on the primary.
func (cs *ClusterStore) Migrate(ctx context.Context, migrations []string) error {
	db := cs.cluster.Primary()
	for _, m := range migrations {
		if _, err := db.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}

// HealthCheck checks cluster health.
func (cs *ClusterStore) HealthCheck(ctx context.Context) error {
	return cs.cluster.HealthCheck(ctx)
}

// splitMigrations is a helper to split SQL by semicolons for cluster migration.
func splitMigrations(sqlStr string) []string {
	var out []string
	for _, s := range strings.Split(sqlStr, ";") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
