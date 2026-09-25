package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SystemEventInput struct {
	Key        string
	Severity   string
	Title      string
	Message    string
	Link       string
	Visibility string
}

type SystemEvent struct {
	ID          int64  `json:"id"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	Link        string `json:"link"`
	OccurredAt  string `json:"occurred_at"`
	RepeatCount int    `json:"repeat_count"`
	Read        bool   `json:"read"`
}

type SystemEventPage struct {
	Events      []SystemEvent `json:"events"`
	UnreadCount int           `json:"unread_count"`
}

var ErrInvalidSystemEvent = errors.New("invalid system event")

func (s *Store) PublishSystemEvent(ctx context.Context, input SystemEventInput) error {
	if len(input.Key) == 0 || len(input.Key) > 160 || len(input.Title) == 0 || len(input.Title) > 120 || len(input.Message) > 500 || strings.ContainsAny(input.Key+input.Title+input.Message, "\x00\r\n") || (input.Severity != "info" && input.Severity != "warning" && input.Severity != "critical") || (input.Visibility != "all" && input.Visibility != "admin") || (input.Link != "" && (!strings.HasPrefix(input.Link, "/") || strings.HasPrefix(input.Link, "//") || len(input.Link) > 160)) {
		return ErrInvalidSystemEvent
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	var id int64
	var occurredAt string
	query := fmt.Sprintf("SELECT id,occurred_at FROM system_events WHERE event_key=%s ORDER BY id DESC LIMIT 1", s.placeholder(1))
	err = tx.QueryRowContext(ctx, query, input.Key).Scan(&id, &occurredAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	last, _ := time.Parse(time.RFC3339Nano, occurredAt)
	if id > 0 && now.Sub(last) < 10*time.Minute {
		update := fmt.Sprintf("UPDATE system_events SET repeat_count=repeat_count+1,occurred_at=%s,severity=%s,title=%s,message=%s,link=%s,visibility=%s WHERE id=%s", s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.placeholder(6), s.placeholder(7))
		if _, err = tx.ExecContext(ctx, update, now.Format(time.RFC3339Nano), input.Severity, input.Title, input.Message, input.Link, input.Visibility, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM system_event_reads WHERE event_id="+s.placeholder(1), id); err != nil {
			return err
		}
	} else {
		insert := fmt.Sprintf("INSERT INTO system_events(event_key,severity,title,message,link,visibility,occurred_at) VALUES(%s,%s,%s,%s,%s,%s,%s)", s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.placeholder(6), s.placeholder(7))
		if _, err = tx.ExecContext(ctx, insert, input.Key, input.Severity, input.Title, input.Message, input.Link, input.Visibility, now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM system_events WHERE occurred_at<"+s.placeholder(1), now.Add(-90*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM system_events WHERE id NOT IN (SELECT id FROM system_events ORDER BY id DESC LIMIT 1000)"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListSystemEvents(ctx context.Context, userID int64, role string, limit int) (SystemEventPage, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	visibility := "visibility='all'"
	if role == "admin" {
		visibility = "(visibility='all' OR visibility='admin')"
	}
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM system_events e WHERE %s AND NOT EXISTS (SELECT 1 FROM system_event_reads r WHERE r.event_id=e.id AND r.user_id=%s)", visibility, s.placeholder(1))
	page := SystemEventPage{Events: []SystemEvent{}}
	if err := s.db.QueryRowContext(ctx, countQuery, userID).Scan(&page.UnreadCount); err != nil {
		return page, err
	}
	listQuery := fmt.Sprintf("SELECT e.id,e.severity,e.title,e.message,e.link,e.occurred_at,e.repeat_count,EXISTS(SELECT 1 FROM system_event_reads r WHERE r.event_id=e.id AND r.user_id=%s) FROM system_events e WHERE %s ORDER BY e.occurred_at DESC,e.id DESC LIMIT %s", s.placeholder(1), visibility, s.placeholder(2))
	rows, err := s.db.QueryContext(ctx, listQuery, userID, limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var event SystemEvent
		if err := rows.Scan(&event.ID, &event.Severity, &event.Title, &event.Message, &event.Link, &event.OccurredAt, &event.RepeatCount, &event.Read); err != nil {
			return page, err
		}
		page.Events = append(page.Events, event)
	}
	return page, rows.Err()
}

func (s *Store) MarkSystemEventRead(ctx context.Context, userID int64, role string, eventID int64) error {
	if eventID < 1 {
		return ErrInvalidSystemEvent
	}
	visibility := "visibility='all'"
	if role == "admin" {
		visibility = "(visibility='all' OR visibility='admin')"
	}
	query := fmt.Sprintf("INSERT INTO system_event_reads(user_id,event_id) SELECT %s,id FROM system_events WHERE id=%s AND %s ON CONFLICT DO NOTHING", s.placeholder(1), s.placeholder(2), visibility)
	result, err := s.db.ExecContext(ctx, query, userID, eventID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists bool
		check := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM system_events WHERE id=%s AND %s)", s.placeholder(1), visibility)
		if err := s.db.QueryRowContext(ctx, check, eventID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return sql.ErrNoRows
		}
	}
	return nil
}
