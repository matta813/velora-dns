package database

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/matta813/velora-dns/internal/policies"
)

var _ policies.Store = (*Store)(nil)

func (s *Store) LoadPolicies(ctx context.Context) ([]policies.Policy, error) {
	query := fmt.Sprintf("SELECT id,client_id,mode,blocklist_ids,allow_domains,block_domains,enabled FROM client_policies ORDER BY client_id LIMIT %s", s.placeholder(1))
	rows, err := s.db.QueryContext(ctx, query, policies.MaxPolicies+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []policies.Policy{}
	for rows.Next() {
		var policy policies.Policy
		var blocklists, allow, block string
		if err = rows.Scan(&policy.ID, &policy.ClientID, &policy.Mode, &blocklists, &allow, &block, &policy.Enabled); err != nil {
			return nil, err
		}
		policy.Blocklists = []int64{}
		for _, raw := range strings.Fields(blocklists) {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("stored policy %d has an invalid blocklist id", policy.ID)
			}
			policy.Blocklists = append(policy.Blocklists, id)
		}
		policy.Allow, policy.Block = strings.Fields(allow), strings.Fields(block)
		out = append(out, policy)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > policies.MaxPolicies {
		return nil, fmt.Errorf("stored policy limit exceeded")
	}
	return out, nil
}

func (s *Store) SavePolicy(ctx context.Context, policy policies.Policy) (policies.Policy, error) {
	p := s.placeholder
	ids := make([]string, 0, len(policy.Blocklists))
	for _, id := range policy.Blocklists {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	args := []any{policy.ClientID, policy.Mode, strings.Join(ids, " "), strings.Join(policy.Allow, " "), strings.Join(policy.Block, " "), policy.Enabled}
	if policy.ID == 0 {
		insertSQL := fmt.Sprintf("INSERT INTO client_policies(client_id,mode,blocklist_ids,allow_domains,block_domains,enabled) VALUES(%s,%s,%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), p(5), p(6), s.insertReturning())
		if s.driver == "postgres" {
			if err := s.db.QueryRowContext(ctx, insertSQL, args...).Scan(&policy.ID); err != nil {
				return policy, uniqueAs(err, policies.ErrExists)
			}
			return policy, nil
		}
		result, err := s.db.ExecContext(ctx, insertSQL, args...)
		if err != nil {
			return policy, uniqueAs(err, policies.ErrExists)
		}
		policy.ID, err = result.LastInsertId()
		return policy, err
	}
	updateSQL := fmt.Sprintf("UPDATE client_policies SET client_id=%s,mode=%s,blocklist_ids=%s,allow_domains=%s,block_domains=%s,enabled=%s WHERE id=%s", p(1), p(2), p(3), p(4), p(5), p(6), p(7))
	result, err := s.db.ExecContext(ctx, updateSQL, append(args, policy.ID)...)
	if err != nil {
		return policy, uniqueAs(err, policies.ErrExists)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return policy, errors.Join(policies.ErrNotFound, err)
	}
	return policy, nil
}

func (s *Store) DeletePolicy(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM client_policies WHERE id=%s", s.placeholder(1)), id)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(policies.ErrNotFound, err)
	}
	return nil
}
