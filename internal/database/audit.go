package database

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AuditEvent struct {
	ID         int64  `json:"id"`
	OccurredAt string `json:"occurred_at"`
	Actor      string `json:"actor"`
	Role       string `json:"role"`
	Action     string `json:"action"`
	Target     string `json:"target"`
	Result     string `json:"result"`
	StatusCode int    `json:"status_code"`
}

type AuditFilter struct {
	Actor  string
	Action string
	Result string
	Before int64
	Limit  int
}

func (s *Store) pruneAudit(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	_, err := s.db.ExecContext(ctx, "DELETE FROM audit_events WHERE occurred_at < "+s.placeholder(1), cutoff)
	return err
}

func (s *Store) BeginAudit(ctx context.Context, userID int64, role, action, target string) (int64, error) {
	if err := s.pruneAudit(ctx); err != nil {
		return 0, err
	}
	query := fmt.Sprintf("INSERT INTO audit_events(user_id,role,action,target,result) VALUES(%s,%s,%s,%s,%s)%s", s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.insertReturning())
	if s.driver == "postgres" {
		var id int64
		err := s.db.QueryRowContext(ctx, query, userID, role, action, target, "pending").Scan(&id)
		return id, err
	}
	result, err := s.db.ExecContext(ctx, query, userID, role, action, target, "pending")
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) CompleteAudit(ctx context.Context, id int64, status int) error {
	result := "failure"
	if status >= 200 && status < 400 {
		result = "success"
	}
	query := fmt.Sprintf("UPDATE audit_events SET result=%s,status_code=%s WHERE id=%s", s.placeholder(1), s.placeholder(2), s.placeholder(3))
	_, err := s.db.ExecContext(ctx, query, result, status, id)
	return err
}

func (s *Store) ListAudit(ctx context.Context, filter AuditFilter) ([]AuditEvent, error) {
	query := "SELECT e.id,e.occurred_at,COALESCE(u.username,'system'),e.role,e.action,COALESCE(NULLIF(e.target,''),e.detail),e.result,e.status_code FROM audit_events e LEFT JOIN users u ON e.user_id=u.id WHERE 1=1"
	var args []any
	add := func(column string, value any) {
		args = append(args, value)
		query += " AND " + column + "=" + s.placeholder(len(args))
	}
	if filter.Actor != "" {
		add("u.username", filter.Actor)
	}
	if filter.Action != "" {
		add("e.action", filter.Action)
	}
	if filter.Result != "" {
		add("e.result", filter.Result)
	}
	if filter.Before > 0 {
		args = append(args, filter.Before)
		query += " AND e.id<" + s.placeholder(len(args))
	}
	limit := filter.Limit
	if limit < 1 || limit > 200 {
		limit = 50
	}
	args = append(args, limit)
	query += " ORDER BY e.id DESC LIMIT " + s.placeholder(len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []AuditEvent{}
	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.ID, &event.OccurredAt, &event.Actor, &event.Role, &event.Action, &event.Target, &event.Result, &event.StatusCode); err != nil {
			return nil, err
		}
		// Legacy events predate structured outcomes.
		if event.Result == "" {
			switch event.Action {
			case "login":
				event.Result = "success"
			case "login_failed":
				event.Result = "failure"
			default:
				event.Result = "unknown"
			}
		}
		if event.Target == "" && strings.HasPrefix(event.Action, "login") {
			event.Target = "authentication"
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
