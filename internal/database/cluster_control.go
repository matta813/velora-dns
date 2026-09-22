package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/matta813/velora-dns/internal/cluster"
)

func (s *Store) SaveClusterState(ctx context.Context, state cluster.State) error {
	if state.ClusterID == "" || state.NodeID == "" || state.NodeName == "" || state.ControlAddress == "" || (state.Role != "leader" && state.Role != "voter") || len(state.CACertificate) == 0 || len(state.Certificate) == 0 || len(state.PrivateKey) == 0 {
		return fmt.Errorf("invalid cluster state")
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = time.Now().UTC()
	}
	p := s.placeholder
	query := fmt.Sprintf("INSERT INTO cluster_state(id,cluster_id,node_id,node_name,control_address,role,ca_certificate,certificate,private_key,created_at) VALUES(1,%s,%s,%s,%s,%s,%s,%s,%s,%s) ON CONFLICT(id) DO UPDATE SET cluster_id=excluded.cluster_id,node_id=excluded.node_id,node_name=excluded.node_name,control_address=excluded.control_address,role=excluded.role,ca_certificate=excluded.ca_certificate,certificate=excluded.certificate,private_key=excluded.private_key,created_at=excluded.created_at", p(1), p(2), p(3), p(4), p(5), p(6), p(7), p(8), p(9))
	_, err := s.db.ExecContext(ctx, query, state.ClusterID, state.NodeID, state.NodeName, state.ControlAddress, state.Role, state.CACertificate, state.Certificate, state.PrivateKey, state.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetClusterState(ctx context.Context) (cluster.State, error) {
	var state cluster.State
	var created string
	err := s.db.QueryRowContext(ctx, "SELECT cluster_id,node_id,node_name,control_address,role,ca_certificate,certificate,private_key,created_at FROM cluster_state WHERE id=1").Scan(&state.ClusterID, &state.NodeID, &state.NodeName, &state.ControlAddress, &state.Role, &state.CACertificate, &state.Certificate, &state.PrivateKey, &created)
	if err != nil {
		return cluster.State{}, err
	}
	state.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return cluster.State{}, fmt.Errorf("parse cluster creation time: %w", err)
	}
	return state, nil
}

func (s *Store) CreateClusterJoinToken(ctx context.Context, digest []byte, expiresAt time.Time) error {
	if len(digest) != 32 || !expiresAt.After(time.Now()) {
		return fmt.Errorf("invalid join token")
	}
	p := s.placeholder
	query := fmt.Sprintf("INSERT INTO cluster_join_tokens(token_hash,expires_at,created_at) VALUES(%s,%s,%s)", p(1), p(2), p(3))
	_, err := s.db.ExecContext(ctx, query, digest, expiresAt.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// ConsumeClusterJoinToken atomically consumes exactly one valid token.
func (s *Store) ConsumeClusterJoinToken(ctx context.Context, digest []byte) error {
	if len(digest) != 32 {
		return fmt.Errorf("invalid join token")
	}
	p := s.placeholder
	query := fmt.Sprintf("UPDATE cluster_join_tokens SET used_at=%s WHERE token_hash=%s AND used_at IS NULL AND expires_at>%s", p(1), p(2), p(3))
	result, err := s.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339Nano), digest, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("join token is invalid, expired, or already used")
	}
	return nil
}

func (s *Store) HasClusterState(ctx context.Context) (bool, error) {
	_, err := s.GetClusterState(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
