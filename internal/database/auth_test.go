package database

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/auth"
)

func TestAuthSessionLifecycleAndLastAdmin(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	service := auth.New(db, time.Hour)
	if err = service.Bootstrap(context.Background(), "admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	user, token, csrf, _, err := service.Login(context.Background(), "ADMIN", "correct horse battery staple")
	if err != nil || user.Role != auth.Admin {
		t.Fatalf("login: %+v %v", user, err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(token)
	var rawStored int
	if err = db.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE token_hash=?", raw).Scan(&rawStored); err != nil || rawStored != 0 {
		t.Fatal("raw session token stored")
	}
	session, err := service.Authenticate(context.Background(), token)
	if err != nil || !service.CSRF(session, csrf) || service.CSRF(session, csrf+"x") {
		t.Fatal("session or CSRF validation")
	}
	if err = service.Logout(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Authenticate(context.Background(), token); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatal("revoked session accepted")
	}
	active := false
	if _, err = db.UpdateUser(context.Background(), user.ID, nil, nil, &active); !errors.Is(err, auth.ErrLastAdmin) {
		t.Fatalf("last admin disabled: %v", err)
	}
	if err = db.DeleteUser(context.Background(), user.ID); !errors.Is(err, auth.ErrLastAdmin) {
		t.Fatalf("last admin deleted: %v", err)
	}
}
