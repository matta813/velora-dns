package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/querylog"
)

var _ clients.Store = (*Store)(nil)

func (s *Store) LoadClients(ctx context.Context) ([]clients.Client, error) {
	query := fmt.Sprintf("SELECT id,name,addresses,client_group,description,enabled FROM clients ORDER BY LOWER(name) LIMIT %s", s.placeholder(1))
	rows, err := s.db.QueryContext(ctx, query, clients.MaxClients+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []clients.Client{}
	for rows.Next() {
		var client clients.Client
		var addresses string
		if err = rows.Scan(&client.ID, &client.Name, &addresses, &client.Group, &client.Description, &client.Enabled); err != nil {
			return nil, err
		}
		client.Addresses = strings.Fields(addresses)
		out = append(out, client)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > clients.MaxClients {
		return nil, fmt.Errorf("stored client limit exceeded")
	}
	return out, nil
}

func (s *Store) SaveClient(ctx context.Context, client clients.Client) (clients.Client, error) {
	p := s.placeholder
	addresses := strings.Join(client.Addresses, " ")
	if client.ID == 0 {
		insertSQL := fmt.Sprintf("INSERT INTO clients(name,addresses,client_group,description,enabled) VALUES(%s,%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), p(5), s.insertReturning())
		args := []any{client.Name, addresses, client.Group, client.Description, client.Enabled}
		if s.driver == "postgres" {
			if err := s.db.QueryRowContext(ctx, insertSQL, args...).Scan(&client.ID); err != nil {
				return client, uniqueAs(err, clients.ErrExists)
			}
			return client, nil
		}
		result, err := s.db.ExecContext(ctx, insertSQL, args...)
		if err != nil {
			return client, uniqueAs(err, clients.ErrExists)
		}
		client.ID, err = result.LastInsertId()
		return client, err
	}
	updateSQL := fmt.Sprintf("UPDATE clients SET name=%s,addresses=%s,client_group=%s,description=%s,enabled=%s WHERE id=%s", p(1), p(2), p(3), p(4), p(5), p(6))
	result, err := s.db.ExecContext(ctx, updateSQL, client.Name, addresses, client.Group, client.Description, client.Enabled, client.ID)
	if err != nil {
		return client, uniqueAs(err, clients.ErrExists)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return client, errors.Join(clients.ErrNotFound, err)
	}
	return client, nil
}

func (s *Store) DeleteClient(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM clients WHERE id=%s", s.placeholder(1)), id)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(clients.ErrNotFound, err)
	}
	return nil
}

// ClientActivity aggregates retained queries per client IP since start. The
// result is bounded; the busiest addresses come first.
func (s *Store) ClientActivity(ctx context.Context, start time.Time, limit int) ([]querylog.ClientActivity, error) {
	if limit < 1 || limit > 10000 {
		return nil, fmt.Errorf("invalid activity limit")
	}
	p := s.placeholder
	query := fmt.Sprintf("SELECT client_ip, COUNT(*) AS frequency, MAX(occurred_at) FROM query_log WHERE occurred_at>=%s GROUP BY client_ip ORDER BY frequency DESC, client_ip ASC LIMIT %s", p(1), p(2))
	rows, err := s.db.QueryContext(ctx, query, start.UTC().Format(queryTimeFormat), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []querylog.ClientActivity{}
	for rows.Next() {
		var item querylog.ClientActivity
		var last string
		if err = rows.Scan(&item.ClientIP, &item.Queries, &last); err != nil {
			return nil, err
		}
		if item.LastSeen, err = time.Parse(queryTimeFormat, last); err != nil {
			return nil, fmt.Errorf("invalid query timestamp: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
