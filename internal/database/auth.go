package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/matta813/velora-dns/internal/auth"
)

func (s *Store) HasUsers(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users)").Scan(&exists)
	return exists, err
}
func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, role auth.Role) (auth.User, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.User{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return auth.User{}, err
	}
	if count >= 256 {
		return auth.User{}, fmt.Errorf("%w: user limit", auth.ErrLimit)
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO users(username,password_hash,role,active,created_at) VALUES(?,?,?,1,?)", username, passwordHash, role, now.Format(queryTimeFormat))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.username") {
			return auth.User{}, fmt.Errorf("%w: create user", auth.ErrConflict)
		}
		return auth.User{}, err
	}
	id, err := result.LastInsertId()
	if err == nil {
		err = tx.Commit()
	}
	return auth.User{ID: id, Username: username, PasswordHash: passwordHash, Role: role, Active: true, CreatedAt: now}, err
}
func scanUser(row interface{ Scan(...any) error }) (auth.User, error) {
	var user auth.User
	var created string
	if err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Active, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, auth.ErrNotFound
		}
		return user, err
	}
	var err error
	user.CreatedAt, err = time.Parse(queryTimeFormat, created)
	return user, err
}
func (s *Store) UserByUsername(ctx context.Context, username string) (auth.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT id,username,password_hash,role,active,created_at FROM users WHERE username=?", username))
}
func (s *Store) ListUsers(ctx context.Context) ([]auth.User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,username,password_hash,role,active,created_at FROM users ORDER BY username COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	users := []auth.User{}
	for rows.Next() {
		user, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		users = append(users, user)
	}
	return users, rows.Err()
}
func (s *Store) UpdateUser(ctx context.Context, id int64, passwordHash *string, role *auth.Role, active *bool) (auth.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.User{}, err
	}
	defer func() { _ = tx.Rollback() }()
	user, err := scanUser(tx.QueryRowContext(ctx, "SELECT id,username,password_hash,role,active,created_at FROM users WHERE id=?", id))
	if err != nil {
		return auth.User{}, err
	}
	newRole, newActive := user.Role, user.Active
	if role != nil {
		newRole = *role
	}
	if active != nil {
		newActive = *active
	}
	if user.Role == auth.Admin && user.Active && (newRole != auth.Admin || !newActive) {
		var admins int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role='admin' AND active=1").Scan(&admins); err != nil {
			return auth.User{}, err
		}
		if admins <= 1 {
			return auth.User{}, auth.ErrLastAdmin
		}
	}
	newHash := user.PasswordHash
	if passwordHash != nil {
		newHash = *passwordHash
	}
	if _, err = tx.ExecContext(ctx, "UPDATE users SET password_hash=?,role=?,active=? WHERE id=?", newHash, newRole, newActive, id); err != nil {
		return auth.User{}, err
	}
	if passwordHash != nil || !newActive {
		if _, err = tx.ExecContext(ctx, "UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL", time.Now().UTC().Format(queryTimeFormat), id); err != nil {
			return auth.User{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return auth.User{}, err
	}
	user.PasswordHash, user.Role, user.Active = newHash, newRole, newActive
	return user, nil
}
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	user, err := scanUser(tx.QueryRowContext(ctx, "SELECT id,username,password_hash,role,active,created_at FROM users WHERE id=?", id))
	if err != nil {
		return err
	}
	if user.Role == auth.Admin && user.Active {
		var admins int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role='admin' AND active=1").Scan(&admins); err != nil {
			return err
		}
		if admins <= 1 {
			return auth.ErrLastAdmin
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM users WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, userID int64, csrfHash []byte, created, expires time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at<=? OR revoked_at IS NOT NULL", created.Format(queryTimeFormat)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,csrf_hash,created_at,expires_at) VALUES(?,?,?,?,?)", tokenHash, userID, csrfHash, created.Format(queryTimeFormat), expires.Format(queryTimeFormat)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=? AND token_hash NOT IN (SELECT token_hash FROM sessions WHERE user_id=? ORDER BY created_at DESC LIMIT 32)", userID, userID); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Session(ctx context.Context, tokenHash []byte, now time.Time) (auth.Session, error) {
	var session auth.Session
	var csrf []byte
	var expires string
	row := s.db.QueryRowContext(ctx, "SELECT u.id,u.username,u.password_hash,u.role,u.active,u.created_at,s.csrf_hash,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.revoked_at IS NULL AND s.expires_at>? AND u.active=1", tokenHash, now.Format(queryTimeFormat))
	var created string
	if err := row.Scan(&session.User.ID, &session.User.Username, &session.User.PasswordHash, &session.User.Role, &session.User.Active, &created, &csrf, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session, auth.ErrInvalidCredentials
		}
		return session, err
	}
	if len(csrf) != 32 {
		return session, fmt.Errorf("invalid stored CSRF hash")
	}
	copy(session.CSRFHash[:], csrf)
	var err error
	session.User.CreatedAt, err = time.Parse(queryTimeFormat, created)
	if err == nil {
		session.ExpiresAt, err = time.Parse(queryTimeFormat, expires)
	}
	return session, err
}
func (s *Store) RevokeSession(ctx context.Context, tokenHash []byte, now time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET revoked_at=? WHERE token_hash=? AND revoked_at IS NULL", now.Format(queryTimeFormat), tokenHash)
	return err
}
func (s *Store) RevokeUserSessions(ctx context.Context, userID int64, now time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL", now.Format(queryTimeFormat), userID)
	return err
}
func (s *Store) AddAudit(ctx context.Context, event auth.AuditEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "INSERT INTO audit_events(occurred_at,actor_user_id,actor_username,action,outcome,remote_ip) VALUES(?,?,?,?,?,?)", event.OccurredAt.Format(queryTimeFormat), event.ActorUserID, event.ActorUsername, event.Action, event.Outcome, event.RemoteIP); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM audit_events WHERE id <= (SELECT COALESCE(MAX(id),0)-10000 FROM audit_events)"); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ListAudit(ctx context.Context, limit int) ([]auth.AuditEvent, error) {
	if limit < 1 || limit > 500 {
		return nil, fmt.Errorf("invalid audit limit")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,occurred_at,actor_user_id,actor_username,action,outcome,remote_ip FROM audit_events ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := []auth.AuditEvent{}
	for rows.Next() {
		var event auth.AuditEvent
		var occurred string
		if err = rows.Scan(&event.ID, &occurred, &event.ActorUserID, &event.ActorUsername, &event.Action, &event.Outcome, &event.RemoteIP); err != nil {
			return nil, err
		}
		event.OccurredAt, err = time.Parse(queryTimeFormat, occurred)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
