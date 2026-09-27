package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/matta813/velora-dns/internal/rewrites"
)

var _ rewrites.Store = (*Store)(nil)

func (s *Store) LoadRewrites(ctx context.Context) ([]rewrites.Rule, error) {
	query := fmt.Sprintf("SELECT id,name,type,value,enabled,description FROM dns_rewrites ORDER BY name,type,value LIMIT %s", s.placeholder(1))
	rows, err := s.db.QueryContext(ctx, query, rewrites.MaxRules+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []rewrites.Rule{}
	for rows.Next() {
		var rule rewrites.Rule
		if err = rows.Scan(&rule.ID, &rule.Name, &rule.Type, &rule.Value, &rule.Enabled, &rule.Description); err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > rewrites.MaxRules {
		return nil, fmt.Errorf("stored rewrite limit exceeded")
	}
	return out, nil
}

func (s *Store) SaveRewrite(ctx context.Context, rule rewrites.Rule) (rewrites.Rule, error) {
	p := s.placeholder
	if rule.ID == 0 {
		insertSQL := fmt.Sprintf("INSERT INTO dns_rewrites(name,type,value,enabled,description) VALUES(%s,%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), p(5), s.insertReturning())
		if s.driver == "postgres" {
			if err := s.db.QueryRowContext(ctx, insertSQL, rule.Name, rule.Type, rule.Value, rule.Enabled, rule.Description).Scan(&rule.ID); err != nil {
				return rule, uniqueAs(err, rewrites.ErrExists)
			}
			return rule, nil
		}
		result, err := s.db.ExecContext(ctx, insertSQL, rule.Name, rule.Type, rule.Value, rule.Enabled, rule.Description)
		if err != nil {
			return rule, uniqueAs(err, rewrites.ErrExists)
		}
		rule.ID, err = result.LastInsertId()
		return rule, err
	}
	updateSQL := fmt.Sprintf("UPDATE dns_rewrites SET name=%s,type=%s,value=%s,enabled=%s,description=%s WHERE id=%s", p(1), p(2), p(3), p(4), p(5), p(6))
	result, err := s.db.ExecContext(ctx, updateSQL, rule.Name, rule.Type, rule.Value, rule.Enabled, rule.Description, rule.ID)
	if err != nil {
		return rule, uniqueAs(err, rewrites.ErrExists)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return rule, errors.Join(rewrites.ErrNotFound, err)
	}
	return rule, nil
}

func (s *Store) DeleteRewrite(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM dns_rewrites WHERE id=%s", s.placeholder(1)), id)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(rewrites.ErrNotFound, err)
	}
	return nil
}
