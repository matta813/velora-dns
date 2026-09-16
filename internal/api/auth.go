package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/matta813/velora-dns/internal/database"
)

const sessionCookie = "velora_session"

type AuthStore interface {
	CreateUser(context.Context, string, string, string) (database.User, error)
	ListUsers(context.Context) ([]database.User, error)
	Authenticate(context.Context, string, string) (database.User, error)
	CreateSession(context.Context, database.User, []byte, []byte, time.Time) error
	Session(context.Context, []byte) (database.User, []byte, error)
	RevokeSession(context.Context, []byte) error
	Audit(context.Context, *int64, string, string) error
}

type authContextKey struct{}
type csrfContextKey struct{}

func registerAuth(mux *http.ServeMux, store AuthStore) {
	mux.HandleFunc("GET /api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		users, err := store.ListUsers(r.Context())
		if err != nil {
			failure(w, 503, "storage_unavailable", "Users unavailable")
			return
		}
		respond(w, 200, users)
	})
	mux.HandleFunc("POST /api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		user, err := store.CreateUser(r.Context(), input.Username, input.Password, input.Role)
		if err != nil {
			failure(w, 400, "invalid_user", "Username, password or role is invalid or already exists")
			return
		}
		respond(w, 201, user)
	})
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		user, err := store.Authenticate(r.Context(), input.Username, input.Password)
		if err != nil {
			_ = store.Audit(r.Context(), nil, "login_failed", "")
			failure(w, http.StatusUnauthorized, "invalid_credentials", "Invalid username or password")
			return
		}
		token, csrf := make([]byte, 32), make([]byte, 32)
		if _, err = rand.Read(token); err != nil {
			failure(w, 503, "session_unavailable", "Session could not be created")
			return
		}
		if _, err = rand.Read(csrf); err != nil {
			failure(w, 503, "session_unavailable", "Session could not be created")
			return
		}
		if err = store.CreateSession(r.Context(), user, token, csrf, time.Now().Add(12*time.Hour)); err != nil {
			failure(w, 503, "session_unavailable", "Session could not be created")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: base64.RawURLEncoding.EncodeToString(token), Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 12 * 60 * 60})
		_ = store.Audit(r.Context(), &user.ID, "login", "")
		respond(w, 200, map[string]any{"username": user.Username, "role": user.Role, "csrf_token": base64.RawURLEncoding.EncodeToString(csrf)})
	})
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(authContextKey{}).(database.User)
		csrf := r.Context().Value(csrfContextKey{}).([]byte)
		respond(w, 200, map[string]any{"username": user.Username, "role": user.Role, "csrf_token": base64.RawURLEncoding.EncodeToString(csrf)})
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie(sessionCookie)
		token, _ := base64.RawURLEncoding.DecodeString(cookie.Value)
		_ = store.RevokeSession(r.Context(), token)
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		respond(w, 200, map[string]bool{"logged_out": true})
	})
}

func authenticate(r *http.Request, store AuthStore) (*http.Request, []byte, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return r, nil, false
	}
	token, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(token) != 32 {
		return r, nil, false
	}
	user, csrf, err := store.Session(r.Context(), token)
	if err != nil {
		return r, nil, false
	}
	ctx := context.WithValue(r.Context(), authContextKey{}, user)
	ctx = context.WithValue(ctx, csrfContextKey{}, csrf)
	return r.WithContext(ctx), csrf, true
}

func validCSRF(header string, expected []byte) bool {
	provided, err := base64.RawURLEncoding.DecodeString(header)
	return err == nil && len(provided) == len(expected) && subtle.ConstantTimeCompare(provided, expected) == 1
}
