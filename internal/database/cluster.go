package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/matta813/velora-dns/internal/cluster"
)

var _ cluster.Store = (*Store)(nil)

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &t
}

func (s *Store) LoadClusterState(ctx context.Context) (cluster.State, bool, error) {
	var state cluster.State
	var created, lastSync string
	err := s.db.QueryRowContext(ctx, "SELECT role,cluster_id,node_id,node_name,advertised_url,primary_url,node_secret,allow_insecure,created_at,last_sync_at,last_sync_error,applied_revision FROM cluster_state WHERE id=1").
		Scan(&state.Role, &state.ClusterID, &state.NodeID, &state.NodeName, &state.AdvertisedURL, &state.PrimaryURL, &state.Secret, &state.AllowInsecure, &created, &lastSync, &state.LastSyncError, &state.AppliedRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return cluster.State{}, false, nil
	}
	if err != nil {
		return cluster.State{}, false, err
	}
	state.CreatedAt, state.LastSyncAt = parseTime(created), parseTime(lastSync)
	return state, true, nil
}

func (s *Store) SaveClusterState(ctx context.Context, state cluster.State) error {
	p := s.placeholder
	query := fmt.Sprintf(`INSERT INTO cluster_state(id,role,cluster_id,node_id,node_name,advertised_url,primary_url,node_secret,allow_insecure,created_at,last_sync_at,last_sync_error,applied_revision)
VALUES(1,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
ON CONFLICT(id) DO UPDATE SET role=excluded.role,cluster_id=excluded.cluster_id,node_id=excluded.node_id,node_name=excluded.node_name,advertised_url=excluded.advertised_url,primary_url=excluded.primary_url,node_secret=excluded.node_secret,allow_insecure=excluded.allow_insecure,created_at=excluded.created_at,last_sync_at=excluded.last_sync_at,last_sync_error=excluded.last_sync_error,applied_revision=excluded.applied_revision`,
		p(1), p(2), p(3), p(4), p(5), p(6), p(7), p(8), p(9), p(10), p(11), p(12))
	_, err := s.db.ExecContext(ctx, query, state.Role, state.ClusterID, state.NodeID, state.NodeName, state.AdvertisedURL, state.PrimaryURL, state.Secret, state.AllowInsecure, formatTime(state.CreatedAt), formatTime(state.LastSyncAt), state.LastSyncError, state.AppliedRevision)
	return err
}

const memberColumns = "node_id,name,address,version,credential_hash,joined_at,last_seen_at,applied_revision,last_error"

func scanMember(row interface{ Scan(...any) error }) (cluster.Member, string, error) {
	var member cluster.Member
	var hash, joined, seen string
	if err := row.Scan(&member.NodeID, &member.Name, &member.Address, &member.Version, &hash, &joined, &seen, &member.AppliedRevision, &member.LastError); err != nil {
		return cluster.Member{}, "", err
	}
	if t := parseTime(joined); t != nil {
		member.JoinedAt = *t
	}
	member.LastSeenAt = parseTime(seen)
	return member, hash, nil
}

func (s *Store) ListClusterMembers(ctx context.Context) ([]cluster.Member, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+memberColumns+" FROM cluster_members ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []cluster.Member{}
	for rows.Next() {
		member, _, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, member)
	}
	return out, rows.Err()
}

func (s *Store) ClusterMember(ctx context.Context, nodeID string) (cluster.Member, string, error) {
	member, hash, err := scanMember(s.db.QueryRowContext(ctx, "SELECT "+memberColumns+" FROM cluster_members WHERE node_id="+s.placeholder(1), nodeID))
	if errors.Is(err, sql.ErrNoRows) {
		return cluster.Member{}, "", cluster.ErrNotFound
	}
	return member, hash, err
}

func (s *Store) SaveClusterMember(ctx context.Context, member cluster.Member, credentialHash string) error {
	p := s.placeholder
	query := fmt.Sprintf("INSERT INTO cluster_members("+memberColumns+") VALUES(%s,%s,%s,%s,%s,%s,%s,%s,%s)", p(1), p(2), p(3), p(4), p(5), p(6), p(7), p(8), p(9))
	if _, err := s.db.ExecContext(ctx, query, member.NodeID, member.Name, member.Address, member.Version, credentialHash, member.JoinedAt.UTC().Format(time.RFC3339Nano), formatTime(member.LastSeenAt), member.AppliedRevision, member.LastError); err != nil {
		return uniqueAs(err, cluster.ErrConflict)
	}
	return nil
}

func (s *Store) TouchClusterMember(ctx context.Context, nodeID string, seen time.Time, revision, lastError, version string) error {
	p := s.placeholder
	query := fmt.Sprintf("UPDATE cluster_members SET last_seen_at=%s,applied_revision=%s,last_error=%s,version=CASE WHEN %s='' THEN version ELSE %s END WHERE node_id=%s", p(1), p(2), p(3), p(4), p(5), p(6))
	result, err := s.db.ExecContext(ctx, query, seen.UTC().Format(time.RFC3339Nano), revision, lastError, version, version, nodeID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(cluster.ErrNotFound, err)
	}
	return nil
}

func (s *Store) DeleteClusterMember(ctx context.Context, nodeID string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM cluster_members WHERE node_id="+s.placeholder(1), nodeID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(cluster.ErrNotFound, err)
	}
	return nil
}

func (s *Store) DeleteClusterMembers(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM cluster_members"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM cluster_join_tokens"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveJoinToken(ctx context.Context, hash string, expires time.Time) error {
	p := s.placeholder
	if _, err := s.db.ExecContext(ctx, "DELETE FROM cluster_join_tokens WHERE expires_at<"+p(1), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("INSERT INTO cluster_join_tokens(token_hash,expires_at) VALUES(%s,%s)", p(1), p(2)), hash, expires.UTC().Format(time.RFC3339Nano))
	return err
}

// ConsumeJoinToken deletes the token in one statement, so two concurrent
// joins cannot both use it.
func (s *Store) ConsumeJoinToken(ctx context.Context, hash string, now time.Time) (bool, error) {
	p := s.placeholder
	result, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM cluster_join_tokens WHERE token_hash=%s AND expires_at>=%s", p(1), p(2)), hash, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
