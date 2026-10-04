package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/matta813/velora-dns/internal/webhooks"
)

var _ webhooks.Store = (*Store)(nil)

func (s *Store) LoadWebhooks(ctx context.Context) ([]webhooks.Hook, error) {
	query := fmt.Sprintf("SELECT id,name,url,event_types,min_severity,allow_private,token,enabled FROM webhooks ORDER BY LOWER(name) LIMIT %s", s.placeholder(1))
	rows, err := s.db.QueryContext(ctx, query, webhooks.MaxHooks+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []webhooks.Hook{}
	for rows.Next() {
		var hook webhooks.Hook
		var events string
		if err = rows.Scan(&hook.ID, &hook.Name, &hook.URL, &events, &hook.MinSeverity, &hook.AllowPrivate, &hook.Token, &hook.Enabled); err != nil {
			return nil, err
		}
		hook.Events = strings.Fields(events)
		out = append(out, hook)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > webhooks.MaxHooks {
		return nil, fmt.Errorf("stored webhook limit exceeded")
	}
	return out, nil
}

func (s *Store) SaveWebhook(ctx context.Context, hook webhooks.Hook) (webhooks.Hook, error) {
	p := s.placeholder
	args := []any{hook.Name, hook.URL, strings.Join(hook.Events, " "), hook.MinSeverity, hook.AllowPrivate, hook.Token, hook.Enabled}
	if hook.ID == 0 {
		insertSQL := fmt.Sprintf("INSERT INTO webhooks(name,url,event_types,min_severity,allow_private,token,enabled) VALUES(%s,%s,%s,%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), p(5), p(6), p(7), s.insertReturning())
		if s.driver == "postgres" {
			if err := s.db.QueryRowContext(ctx, insertSQL, args...).Scan(&hook.ID); err != nil {
				return hook, uniqueAs(err, webhooks.ErrExists)
			}
			return hook, nil
		}
		result, err := s.db.ExecContext(ctx, insertSQL, args...)
		if err != nil {
			return hook, uniqueAs(err, webhooks.ErrExists)
		}
		hook.ID, err = result.LastInsertId()
		return hook, err
	}
	updateSQL := fmt.Sprintf("UPDATE webhooks SET name=%s,url=%s,event_types=%s,min_severity=%s,allow_private=%s,token=%s,enabled=%s WHERE id=%s", p(1), p(2), p(3), p(4), p(5), p(6), p(7), p(8))
	result, err := s.db.ExecContext(ctx, updateSQL, append(args, hook.ID)...)
	if err != nil {
		return hook, uniqueAs(err, webhooks.ErrExists)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return hook, errors.Join(webhooks.ErrNotFound, err)
	}
	return hook, nil
}

func (s *Store) DeleteWebhook(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM webhooks WHERE id=%s", s.placeholder(1)), id)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(webhooks.ErrNotFound, err)
	}
	return nil
}
