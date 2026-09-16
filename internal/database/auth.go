package database

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var ErrAuthentication = errors.New("authentication failed")
var dummyPasswordHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("not-a-real-management-password"), bcrypt.DefaultCost)
	if err != nil {
		panic("initialize bcrypt comparison hash")
	}
	return hash
}()

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,username,role FROM users WHERE disabled=0 ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var users []User
	for rows.Next() {
		var u User
		if err = rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, username, password, role string) (User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 64 || len(password) < 12 || !validRole(role) {
		return User{}, fmt.Errorf("invalid user input")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	result, err := s.db.ExecContext(ctx, "INSERT INTO users(username,password_hash,role) VALUES(?,?,?)", username, string(hash), role)
	if err != nil {
		return User{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, Role: role}, nil
}

func (s *Store) Authenticate(ctx context.Context, username, password string) (User, error) {
	var u User
	var hash string
	var disabled bool
	err := s.db.QueryRowContext(ctx, "SELECT id,username,password_hash,role,disabled FROM users WHERE username=?", strings.TrimSpace(username)).Scan(&u.ID, &u.Username, &hash, &u.Role, &disabled)
	comparisonHash := []byte(hash)
	if err != nil {
		comparisonHash = dummyPasswordHash
	}
	if bcrypt.CompareHashAndPassword(comparisonHash, []byte(password)) != nil || err != nil || disabled {
		return User{}, ErrAuthentication
	}
	return u, nil
}

func (s *Store) CreateSession(ctx context.Context, u User, token, csrf []byte, expires time.Time) error {
	hash := sha256.Sum256(token)
	_, err := s.db.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at) VALUES(?,?,?,?)", hash[:], u.ID, csrf, expires.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) Session(ctx context.Context, token []byte) (User, []byte, error) {
	hash := sha256.Sum256(token)
	var u User
	var csrf []byte
	var expires string
	err := s.db.QueryRowContext(ctx, "SELECT u.id,u.username,u.role,s.csrf_token,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND u.disabled=0", hash[:]).Scan(&u.ID, &u.Username, &u.Role, &csrf, &expires)
	if err != nil {
		return User{}, nil, ErrAuthentication
	}
	t, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !t.After(time.Now()) {
		return User{}, nil, ErrAuthentication
	}
	return u, csrf, nil
}

func (s *Store) RevokeSession(ctx context.Context, token []byte) error {
	hash := sha256.Sum256(token)
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", hash[:])
	return err
}

func (s *Store) Audit(ctx context.Context, userID *int64, action, detail string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO audit_events(user_id,action,detail) VALUES(?,?,?)", userID, action, detail)
	return err
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count)
	return count, err
}

func validRole(role string) bool { return role == "admin" || role == "operator" || role == "viewer" }
