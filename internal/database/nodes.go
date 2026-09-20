package database

import (
	"context"
	"fmt"
	"time"

	"github.com/matta813/velora-dns/internal/node"
)

func (s *Store) SaveNode(ctx context.Context, n node.Node) error {
	p := s.placeholder
	insertSQL := fmt.Sprintf("INSERT INTO nodes(id,name,address,capabilities,version,status,last_seen_at) VALUES(%s,%s,%s,%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), p(5), p(6), p(7), s.insertReturning())
	if s.driver == "postgres" {
		_, err := s.db.ExecContext(ctx, insertSQL, n.ID, n.Name, n.Address, fmt.Sprintf("%v", n.Capabilities), n.Version, n.Status, n.LastSeenAt.UTC().Format(time.RFC3339))
		return err
	}
	_, err := s.db.ExecContext(ctx, insertSQL, n.ID, n.Name, n.Address, fmt.Sprintf("%v", n.Capabilities), n.Version, n.Status, n.LastSeenAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetNode(ctx context.Context, id string) (node.Node, error) {
	p := s.placeholder
	query := fmt.Sprintf("SELECT id,name,address,capabilities,version,status,last_seen_at FROM nodes WHERE id=%s", p(1))
	var n node.Node
	err := s.db.QueryRowContext(ctx, query, id).Scan(&n.ID, &n.Name, &n.Address, &n.Capabilities, &n.Version, &n.Status, &n.LastSeenAt)
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
		if err = rows.Scan(&n.ID, &n.Name, &n.Address, &n.Capabilities, &n.Version, &n.Status, &n.LastSeenAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (s *Store) DeleteNode(ctx context.Context, id string) error {
	p := s.placeholder
	query := fmt.Sprintf("DELETE FROM nodes WHERE id=%s", p(1))
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}
