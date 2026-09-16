package database

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestUsersAndRevocableSessions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user, err := store.CreateUser(ctx, "admin", "correct horse battery staple", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Authenticate(ctx, "admin", "wrong password"); !errors.Is(err, ErrAuthentication) {
		t.Fatal("wrong password accepted")
	}
	authenticated, err := store.Authenticate(ctx, "ADMIN", "correct horse battery staple")
	if err != nil || authenticated.ID != user.ID {
		t.Fatalf("authentication: %+v %v", authenticated, err)
	}
	token, csrf := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	if err = store.CreateSession(ctx, user, token, csrf, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, gotCSRF, err := store.Session(ctx, token)
	if err != nil || got.ID != user.ID || !bytes.Equal(gotCSRF, csrf) {
		t.Fatalf("session: %+v %x %v", got, gotCSRF, err)
	}
	if err = store.RevokeSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Session(ctx, token); !errors.Is(err, ErrAuthentication) {
		t.Fatal("revoked session accepted")
	}
}

func TestRejectsWeakUsersAndExpiredSessions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, "sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err = store.CreateUser(ctx, "a", "short", "root"); err == nil {
		t.Fatal("invalid user accepted")
	}
	user, err := store.CreateUser(ctx, "viewer", "long enough password", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	token := bytes.Repeat([]byte{3}, 32)
	if err = store.CreateSession(ctx, user, token, token, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Session(ctx, token); !errors.Is(err, ErrAuthentication) {
		t.Fatal("expired session accepted")
	}
}
