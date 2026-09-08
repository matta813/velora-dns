// Package auth owns management identities, sessions, authorization, and audit contracts.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

type Role string

const (
	Admin    Role = "admin"
	Operator Role = "operator"
	Viewer   Role = "viewer"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrLastAdmin = errors.New("at least one active administrator is required")
var ErrLimit = errors.New("resource limit reached")

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Role         Role      `json:"role"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	PasswordHash string    `json:"-"`
}
type Session struct {
	User      User
	CSRFHash  [32]byte
	ExpiresAt time.Time
}
type AuditEvent struct {
	ID            int64     `json:"id"`
	OccurredAt    time.Time `json:"occurred_at"`
	ActorUserID   *int64    `json:"actor_user_id,omitempty"`
	ActorUsername string    `json:"actor_username"`
	Action        string    `json:"action"`
	Outcome       string    `json:"outcome"`
	RemoteIP      string    `json:"remote_ip"`
}
type Store interface {
	HasUsers(context.Context) (bool, error)
	CreateUser(context.Context, string, string, Role) (User, error)
	UserByUsername(context.Context, string) (User, error)
	ListUsers(context.Context) ([]User, error)
	UpdateUser(context.Context, int64, *string, *Role, *bool) (User, error)
	DeleteUser(context.Context, int64) error
	CreateSession(context.Context, []byte, int64, []byte, time.Time, time.Time) error
	Session(context.Context, []byte, time.Time) (Session, error)
	RevokeSession(context.Context, []byte, time.Time) error
	RevokeUserSessions(context.Context, int64, time.Time) error
	AddAudit(context.Context, AuditEvent) error
	ListAudit(context.Context, int) ([]AuditEvent, error)
}
type Service struct {
	store Store
	now   func() time.Time
	ttl   time.Duration
}

func New(store Store, ttl time.Duration) *Service {
	return &Service{store: store, now: time.Now, ttl: ttl}
}

func (s *Service) Bootstrap(ctx context.Context, username, password string) error {
	has, err := s.store.HasUsers(ctx)
	if err != nil || has {
		return err
	}
	if err = ValidateUsername(username); err != nil {
		return fmt.Errorf("bootstrap username: %w", err)
	}
	if err = ValidatePassword(password); err != nil {
		return fmt.Errorf("bootstrap password: %w", err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.store.CreateUser(ctx, username, hash, Admin)
	return err
}

func (s *Service) Login(ctx context.Context, username, password string) (User, string, string, time.Time, error) {
	if len(username) > 64 || len(password) > 1024 {
		_, _ = HashPassword("fixed invalid credential input")
		return User{}, "", "", time.Time{}, ErrInvalidCredentials
	}
	user, err := s.store.UserByUsername(ctx, strings.TrimSpace(username))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return User{}, "", "", time.Time{}, err
	}
	passwordValid := err == nil && VerifyPassword(user.PasswordHash, password)
	valid := passwordValid && user.Active
	if !valid {
		// Equalize unknown-user work without retaining or logging submitted credentials.
		if errors.Is(err, ErrNotFound) {
			_, _ = HashPassword(password)
		}
		return User{}, "", "", time.Time{}, ErrInvalidCredentials
	}
	token, tokenHash, err := randomToken()
	if err != nil {
		return User{}, "", "", time.Time{}, err
	}
	csrf, csrfHash, err := randomToken()
	if err != nil {
		return User{}, "", "", time.Time{}, err
	}
	now := s.now().UTC()
	expires := now.Add(s.ttl)
	if err = s.store.CreateSession(ctx, tokenHash[:], user.ID, csrfHash[:], now, expires); err != nil {
		return User{}, "", "", time.Time{}, err
	}
	return user, token, csrf, expires, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return Session{}, ErrInvalidCredentials
	}
	hash := sha256.Sum256(raw)
	return s.store.Session(ctx, hash[:], s.now().UTC())
}
func (s *Service) CSRF(session Session, token string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return false
	}
	hash := sha256.Sum256(raw)
	return subtle.ConstantTimeCompare(hash[:], session.CSRFHash[:]) == 1
}
func (s *Service) Logout(ctx context.Context, token string) error {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil
	}
	hash := sha256.Sum256(raw)
	return s.store.RevokeSession(ctx, hash[:], s.now().UTC())
}
func (s *Service) Store() Store   { return s.store }
func (s *Service) Now() time.Time { return s.now().UTC() }

func ValidateRole(role Role) bool { return role == Admin || role == Operator || role == Viewer }
func ValidateUsername(value string) error {
	if len(value) < 3 || len(value) > 64 || strings.Trim(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-") != "" {
		return fmt.Errorf("username must be 3–64 ASCII letters, digits, dot, underscore or hyphen")
	}
	return nil
}
func ValidatePassword(value string) error {
	if len(value) < 12 || len(value) > 1024 {
		return fmt.Errorf("password must be 12–1024 bytes")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return "$argon2id$v=19$m=65536,t=3,p=2$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory uint32
	var iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil || memory != 65536 || iterations != 3 || threads != 2 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(salt) != 16 || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
func randomToken() (string, [32]byte, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", [32]byte{}, err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), sha256.Sum256(raw[:]), nil
}

func ParseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, ErrNotFound
	}
	return id, nil
}
