package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/node"
)

func (s *Store) SaveNode(ctx context.Context, n node.Node) error {
	p := s.placeholder
	capabilities, err := json.Marshal(n.Capabilities)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("INSERT INTO nodes(id,name,address,capabilities,version,status,last_seen_at) VALUES(%s,%s,%s,%s,%s,%s,%s) ON CONFLICT(id) DO UPDATE SET name=excluded.name,address=excluded.address,capabilities=excluded.capabilities,version=excluded.version,status=excluded.status,last_seen_at=excluded.last_seen_at", p(1), p(2), p(3), p(4), p(5), p(6), p(7))
	_, err = s.db.ExecContext(ctx, query, n.ID, n.Name, n.Address, string(capabilities), n.Version, n.Status, n.LastSeenAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetNode(ctx context.Context, id string) (node.Node, error) {
	p := s.placeholder
	query := fmt.Sprintf("SELECT id,name,address,capabilities,version,status,last_seen_at FROM nodes WHERE id=%s", p(1))
	var n node.Node
	var capabilities, lastSeen string
	err := s.db.QueryRowContext(ctx, query, id).Scan(&n.ID, &n.Name, &n.Address, &capabilities, &n.Version, &n.Status, &lastSeen)
	if err == nil {
		err = decodeNodeFields(&n, capabilities, lastSeen)
	}
	return n, err
}

func (s *Store) ListNodes(ctx context.Context) ([]node.Node, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,address,capabilities,version,status,last_seen_at FROM nodes ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var nodes []node.Node
	for rows.Next() {
		var n node.Node
		var capabilities, lastSeen string
		if err = rows.Scan(&n.ID, &n.Name, &n.Address, &capabilities, &n.Version, &n.Status, &lastSeen); err != nil {
			return nil, err
		}
		if err = decodeNodeFields(&n, capabilities, lastSeen); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func decodeNodeFields(n *node.Node, capabilities, lastSeen string) error {
	if err := json.Unmarshal([]byte(capabilities), &n.Capabilities); err != nil {
		// Existing records used Go's slice formatting rather than JSON.
		n.Capabilities = strings.Fields(strings.Trim(capabilities, "[]"))
	}
	var err error
	n.LastSeenAt, err = time.Parse(time.RFC3339Nano, lastSeen)
	if err != nil {
		n.LastSeenAt, err = time.Parse("2006-01-02 15:04:05", lastSeen)
	}
	return err
}

func (s *Store) DeleteNode(ctx context.Context, id string) error {
	p := s.placeholder
	query := fmt.Sprintf("DELETE FROM nodes WHERE id=%s", p(1))
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}
