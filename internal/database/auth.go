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
type APIToken struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Scopes    string    `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
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
	p := s.placeholder
	insertSQL := fmt.Sprintf("INSERT INTO users(username,password_hash,role) VALUES(%s,%s,%s)%s", p(1), p(2), p(3), s.insertReturning())
	if s.driver == "postgres" {
		var id int64
		if err = s.db.QueryRowContext(ctx, insertSQL, username, string(hash), role).Scan(&id); err != nil {
			return User{}, err
		}
		return User{ID: id, Username: username, Role: role}, nil
	}
	result, err := s.db.ExecContext(ctx, insertSQL, username, string(hash), role)
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
	p := s.placeholder
	query := fmt.Sprintf("SELECT id,username,password_hash,role,disabled FROM users WHERE username=%s", p(1))
	err := s.db.QueryRowContext(ctx, query, strings.TrimSpace(username)).Scan(&u.ID, &u.Username, &hash, &u.Role, &disabled)
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
	p := s.placeholder
	insertSQL := fmt.Sprintf("INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at) VALUES(%s,%s,%s,%s)", p(1), p(2), p(3), p(4))
	_, err := s.db.ExecContext(ctx, insertSQL, hash[:], u.ID, csrf, expires.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) Session(ctx context.Context, token []byte) (User, []byte, error) {
	hash := sha256.Sum256(token)
	var u User
	var csrf []byte
	var expires string
	p := s.placeholder
	query := fmt.Sprintf("SELECT u.id,u.username,u.role,s.csrf_token,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=%s AND u.disabled=0", p(1))
	err := s.db.QueryRowContext(ctx, query, hash[:]).Scan(&u.ID, &u.Username, &u.Role, &csrf, &expires)
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
	p := s.placeholder
	query := fmt.Sprintf("DELETE FROM sessions WHERE token_hash=%s", p(1))
	_, err := s.db.ExecContext(ctx, query, hash[:])
	return err
}

func (s *Store) Audit(ctx context.Context, userID *int64, action, detail string) error {
	p := s.placeholder
	insertSQL := fmt.Sprintf("INSERT INTO audit_events(user_id,action,detail) VALUES(%s,%s,%s)", p(1), p(2), p(3))
	_, err := s.db.ExecContext(ctx, insertSQL, userID, action, detail)
	return err
}

func (s *Store) CreateAPIToken(ctx context.Context, userID int64, name, scopes string, raw []byte, expires time.Time) (APIToken, error) {
	name, scopes = strings.TrimSpace(name), strings.TrimSpace(scopes)
	if name == "" || len(name) > 64 || !expires.After(time.Now()) || !validScopes(scopes) {
		return APIToken{}, fmt.Errorf("invalid API token")
	}
	hash := sha256.Sum256(raw)
	p := s.placeholder
	insertSQL := fmt.Sprintf("INSERT INTO api_tokens(name,token_hash,scopes,expires_at,created_by) VALUES(%s,%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), p(5), s.insertReturning())
	if s.driver == "postgres" {
		var id int64
		if err := s.db.QueryRowContext(ctx, insertSQL, name, hash[:], scopes, expires.UTC().Format(time.RFC3339Nano), userID).Scan(&id); err != nil {
			return APIToken{}, err
		}
		return APIToken{ID: id, Name: name, Scopes: scopes, ExpiresAt: expires.UTC()}, nil
	}
	result, err := s.db.ExecContext(ctx, insertSQL, name, hash[:], scopes, expires.UTC().Format(time.RFC3339Nano), userID)
	if err != nil {
		return APIToken{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return APIToken{}, err
	}
	return APIToken{ID: id, Name: name, Scopes: scopes, ExpiresAt: expires.UTC()}, nil
}

func (s *Store) AuthenticateAPIToken(ctx context.Context, raw []byte) (User, APIToken, error) {
	hash := sha256.Sum256(raw)
	var user User
	var token APIToken
	var expires string
	p := s.placeholder
	query := fmt.Sprintf("SELECT u.id,u.username,u.role,t.id,t.name,t.scopes,t.expires_at FROM api_tokens t JOIN users u ON u.id=t.created_by WHERE t.token_hash=%s AND t.revoked_at IS NULL AND u.disabled=0", p(1))
	err := s.db.QueryRowContext(ctx, query, hash[:]).Scan(&user.ID, &user.Username, &user.Role, &token.ID, &token.Name, &token.Scopes, &expires)
	if err != nil {
		return User{}, APIToken{}, ErrAuthentication
	}
	token.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil || !token.ExpiresAt.After(time.Now()) {
		return User{}, APIToken{}, ErrAuthentication
	}
	return user, token, nil
}

func (s *Store) RevokeAPIToken(ctx context.Context, id int64, userID int64) error {
	p := s.placeholder
	query := fmt.Sprintf("UPDATE api_tokens SET revoked_at=CURRENT_TIMESTAMP WHERE id=%s AND created_by=%s AND revoked_at IS NULL", p(1), p(2))
	result, err := s.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrAuthentication
	}
	return nil
}

func validScopes(scopes string) bool {
	if scopes == "" {
		return false
	}
	for _, scope := range strings.Split(scopes, ",") {
		if scope != "read" && scope != "write" && scope != "admin" {
			return false
		}
	}
	return true
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count)
	return count, err
}

func (s *Store) GetLanguage(ctx context.Context, userID int64) (string, error) {
	var language string
	p := s.placeholder
	query := fmt.Sprintf("SELECT language FROM users WHERE id=%s", p(1))
	if err := s.db.QueryRowContext(ctx, query, userID).Scan(&language); err != nil {
		return "", err
	}
	if language == "" {
		language = "en"
	}
	return language, nil
}

func (s *Store) SetLanguage(ctx context.Context, userID int64, language string) error {
	if !validLanguage(language) {
		return fmt.Errorf("invalid language")
	}
	p := s.placeholder
	query := fmt.Sprintf("UPDATE users SET language=%s WHERE id=%s", p(1), p(2))
	result, err := s.db.ExecContext(ctx, query, language, userID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrAuthentication
	}
	return nil
}

func validLanguage(language string) bool { return language == "en" || language == "de" }

func (s *Store) GetTheme(ctx context.Context, userID int64) (string, error) {
	var theme string
	p := s.placeholder
	query := fmt.Sprintf("SELECT theme FROM users WHERE id=%s", p(1))
	if err := s.db.QueryRowContext(ctx, query, userID).Scan(&theme); err != nil {
		return "", err
	}
	if theme == "" {
		theme = "auto"
	}
	return theme, nil
}

func (s *Store) SetTheme(ctx context.Context, userID int64, theme string) error {
	if !validTheme(theme) {
		return fmt.Errorf("invalid theme")
	}
	p := s.placeholder
	query := fmt.Sprintf("UPDATE users SET theme=%s WHERE id=%s", p(1), p(2))
	result, err := s.db.ExecContext(ctx, query, theme, userID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrAuthentication
	}
	return nil
}

func validTheme(theme string) bool { return theme == "light" || theme == "dark" || theme == "auto" }

func validRole(role string) bool { return role == "admin" || role == "operator" || role == "viewer" }
