package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
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
	CreateAPIToken(context.Context, int64, string, string, []byte, time.Time) (database.APIToken, error)
	AuthenticateAPIToken(context.Context, []byte) (database.User, database.APIToken, error)
	RevokeAPIToken(context.Context, int64, int64) error
	GetLanguage(context.Context, int64) (string, error)
	SetLanguage(context.Context, int64, string) error
}

type authContextKey struct{}
type csrfContextKey struct{}
type tokenContextKey struct{}

func registerAuth(mux *http.ServeMux, store AuthStore) {
	mux.HandleFunc("POST /api/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name           string   `json:"name"`
			Scopes         []string `json:"scopes"`
			ExpiresInHours int      `json:"expires_in_hours"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if input.ExpiresInHours < 1 || input.ExpiresInHours > 24*365 {
			failure(w, 400, "invalid_token", "Expiration must be 1–8760 hours")
			return
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			failure(w, 503, "token_unavailable", "Token could not be created")
			return
		}
		user := r.Context().Value(authContextKey{}).(database.User)
		token, err := store.CreateAPIToken(r.Context(), user.ID, input.Name, strings.Join(input.Scopes, ","), raw, time.Now().Add(time.Duration(input.ExpiresInHours)*time.Hour))
		if err != nil {
			failure(w, 400, "invalid_token", "Token name, scopes or expiration is invalid")
			return
		}
		respond(w, 201, map[string]any{"id": token.ID, "name": token.Name, "scopes": strings.Split(token.Scopes, ","), "expires_at": token.ExpiresAt, "token": "velora_" + base64.RawURLEncoding.EncodeToString(raw)})
	})
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Token ID must be positive")
			return
		}
		user := r.Context().Value(authContextKey{}).(database.User)
		if err = store.RevokeAPIToken(r.Context(), id, user.ID); err != nil {
			failure(w, 404, "not_found", "Token not found")
			return
		}
		respond(w, 200, map[string]int64{"revoked": id})
	})
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
		language, langErr := store.GetLanguage(r.Context(), user.ID)
		if langErr != nil {
			language = "en"
		}
		respond(w, 200, map[string]any{"username": user.Username, "role": user.Role, "csrf_token": base64.RawURLEncoding.EncodeToString(csrf), "language": language})
	})
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(authContextKey{}).(database.User)
		csrf, _ := r.Context().Value(csrfContextKey{}).([]byte)
		language, langErr := store.GetLanguage(r.Context(), user.ID)
		if langErr != nil {
			language = "en"
		}
		respond(w, 200, map[string]any{"username": user.Username, "role": user.Role, "csrf_token": base64.RawURLEncoding.EncodeToString(csrf), "language": language})
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			failure(w, 400, "session_required", "Logout requires a browser session")
			return
		}
		token, _ := base64.RawURLEncoding.DecodeString(cookie.Value)
		_ = store.RevokeSession(r.Context(), token)
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		respond(w, 200, map[string]bool{"logged_out": true})
	})
	mux.HandleFunc("GET /api/v1/preferences", func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(authContextKey{}).(database.User)
		language, err := store.GetLanguage(r.Context(), user.ID)
		if err != nil {
			failure(w, 503, "storage_unavailable", "Preferences unavailable")
			return
		}
		respond(w, 200, map[string]string{"language": language})
	})
	mux.HandleFunc("PUT /api/v1/preferences", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Language string `json:"language"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		user := r.Context().Value(authContextKey{}).(database.User)
		if err := store.SetLanguage(r.Context(), user.ID, input.Language); err != nil {
			failure(w, 400, "invalid_language", "Language must be en or de")
			return
		}
		respond(w, 200, map[string]string{"language": input.Language})
	})
}

func authenticate(r *http.Request, store AuthStore) (*http.Request, []byte, bool) {
	if value := r.Header.Get("Authorization"); strings.HasPrefix(value, "Bearer velora_") {
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "Bearer velora_"))
		if err != nil || len(raw) != 32 {
			return r, nil, false
		}
		user, token, err := store.AuthenticateAPIToken(r.Context(), raw)
		if err != nil {
			return r, nil, false
		}
		ctx := context.WithValue(r.Context(), authContextKey{}, user)
		ctx = context.WithValue(ctx, tokenContextKey{}, token)
		return r.WithContext(ctx), nil, true
	}
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
