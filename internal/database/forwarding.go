package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/matta813/velora-dns/internal/forwarding"
)

var _ forwarding.Store = (*Store)(nil)

func (s *Store) LoadForwardRules(ctx context.Context) ([]forwarding.Rule, error) {
	query := fmt.Sprintf("SELECT id,domain,upstreams,enabled,description FROM forward_rules ORDER BY domain LIMIT %s", s.placeholder(1))
	rows, err := s.db.QueryContext(ctx, query, forwarding.MaxRules+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []forwarding.Rule{}
	for rows.Next() {
		var rule forwarding.Rule
		var upstreams string
		if err = rows.Scan(&rule.ID, &rule.Domain, &upstreams, &rule.Enabled, &rule.Description); err != nil {
			return nil, err
		}
		rule.Upstreams = strings.Fields(upstreams)
		out = append(out, rule)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > forwarding.MaxRules {
		return nil, fmt.Errorf("stored forwarding rule limit exceeded")
	}
	return out, nil
}

func (s *Store) SaveForwardRule(ctx context.Context, rule forwarding.Rule) (forwarding.Rule, error) {
	p := s.placeholder
	upstreams := strings.Join(rule.Upstreams, " ")
	if rule.ID == 0 {
		insertSQL := fmt.Sprintf("INSERT INTO forward_rules(domain,upstreams,enabled,description) VALUES(%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), s.insertReturning())
		if s.driver == "postgres" {
			if err := s.db.QueryRowContext(ctx, insertSQL, rule.Domain, upstreams, rule.Enabled, rule.Description).Scan(&rule.ID); err != nil {
				return rule, s.forwardConstraint(err)
			}
			return rule, nil
		}
		result, err := s.db.ExecContext(ctx, insertSQL, rule.Domain, upstreams, rule.Enabled, rule.Description)
		if err != nil {
			return rule, s.forwardConstraint(err)
		}
		rule.ID, err = result.LastInsertId()
		return rule, err
	}
	updateSQL := fmt.Sprintf("UPDATE forward_rules SET domain=%s,upstreams=%s,enabled=%s,description=%s WHERE id=%s", p(1), p(2), p(3), p(4), p(5))
	result, err := s.db.ExecContext(ctx, updateSQL, rule.Domain, upstreams, rule.Enabled, rule.Description, rule.ID)
	if err != nil {
		return rule, s.forwardConstraint(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return rule, errors.Join(forwarding.ErrNotFound, err)
	}
	return rule, nil
}

func (s *Store) DeleteForwardRule(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM forward_rules WHERE id=%s", s.placeholder(1)), id)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(forwarding.ErrNotFound, err)
	}
	return nil
}

// forwardConstraint maps unique-domain violations to ErrExists for both drivers.
func (s *Store) forwardConstraint(err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique") || strings.Contains(message, "duplicate key") {
		return forwarding.ErrExists
	}
	return err
}
