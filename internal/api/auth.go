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
	BeginAudit(context.Context, int64, string, string, string) (int64, error)
	CompleteAudit(context.Context, int64, int) error
	ListAudit(context.Context, database.AuditFilter) ([]database.AuditEvent, error)
	CreateAPIToken(context.Context, int64, string, string, []byte, time.Time) (database.APIToken, error)
	AuthenticateAPIToken(context.Context, []byte) (database.User, database.APIToken, error)
	RevokeAPIToken(context.Context, int64, int64) error
	GetLanguage(context.Context, int64) (string, error)
	SetLanguage(context.Context, int64, string) error
	GetTheme(context.Context, int64) (string, error)
	SetTheme(context.Context, int64, string) error
	DisableUser(context.Context, int64) error
	UpdateUserPassword(context.Context, int64, string) error
	UpdateUserRole(context.Context, int64, string) error
}

type authContextKey struct{}
type csrfContextKey struct{}
type tokenContextKey struct{}

func registerAuth(mux *http.ServeMux, store AuthStore) {
	mux.HandleFunc("GET /api/v1/audit", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		filter := database.AuditFilter{Actor: query.Get("actor"), Action: query.Get("action"), Result: query.Get("result"), Limit: 50}
		if len(filter.Actor) > 64 || len(filter.Action) > 64 || (filter.Result != "" && filter.Result != "success" && filter.Result != "failure" && filter.Result != "pending" && filter.Result != "unknown") {
			failure(w, 400, "invalid_filter", "Invalid audit filter")
			return
		}
		if raw := query.Get("limit"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 200 {
				failure(w, 400, "invalid_filter", "Audit limit must be 1–200")
				return
			}
			filter.Limit = value
		}
		if raw := query.Get("before"); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 1 {
				failure(w, 400, "invalid_filter", "Audit cursor must be positive")
				return
			}
			filter.Before = value
		}
		events, err := store.ListAudit(r.Context(), filter)
		if err != nil {
			failure(w, 503, "storage_unavailable", "Audit log is unavailable")
			return
		}
		respond(w, 200, events)
	})
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
	mux.HandleFunc("DELETE /api/v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Invalid user ID")
			return
		}
		caller := r.Context().Value(authContextKey{}).(database.User)
		if caller.ID == id {
			failure(w, 400, "cannot_disable_self", "Cannot disable your own account")
			return
		}
		if err := store.DisableUser(r.Context(), id); err != nil {
			failure(w, 404, "not_found", "User not found")
			return
		}
		respond(w, 200, map[string]int64{"disabled": id})
	})
	mux.HandleFunc("PUT /api/v1/users/{id}/password", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Invalid user ID")
			return
		}
		var input struct {
			Password string `json:"password"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if err := store.UpdateUserPassword(r.Context(), id, input.Password); err != nil {
			failure(w, 400, "invalid_password", "Password must be at least 12 characters or user not found")
			return
		}
		respond(w, 200, map[string]string{"status": "updated"})
	})
	mux.HandleFunc("PUT /api/v1/users/{id}/role", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Invalid user ID")
			return
		}
		caller := r.Context().Value(authContextKey{}).(database.User)
		if caller.ID == id {
			failure(w, 400, "cannot_change_own_role", "Cannot change your own role")
			return
		}
		var input struct {
			Role string `json:"role"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if err := store.UpdateUserRole(r.Context(), id, input.Role); err != nil {
			failure(w, 400, "invalid_role", "Role must be admin, operator, or viewer")
			return
		}
		respond(w, 200, map[string]string{"status": "updated"})
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
		theme, themeErr := store.GetTheme(r.Context(), user.ID)
		if themeErr != nil {
			theme = "auto"
		}
		respond(w, 200, map[string]any{"username": user.Username, "role": user.Role, "csrf_token": base64.RawURLEncoding.EncodeToString(csrf), "language": language, "theme": theme})
	})
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(authContextKey{}).(database.User)
		csrf, _ := r.Context().Value(csrfContextKey{}).([]byte)
		language, langErr := store.GetLanguage(r.Context(), user.ID)
		if langErr != nil {
			language = "en"
		}
		theme, themeErr := store.GetTheme(r.Context(), user.ID)
		if themeErr != nil {
			theme = "auto"
		}
		respond(w, 200, map[string]any{"username": user.Username, "role": user.Role, "csrf_token": base64.RawURLEncoding.EncodeToString(csrf), "language": language, "theme": theme})
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
		language, langErr := store.GetLanguage(r.Context(), user.ID)
		if langErr != nil {
			failure(w, 503, "storage_unavailable", "Preferences unavailable")
			return
		}
		theme, themeErr := store.GetTheme(r.Context(), user.ID)
		if themeErr != nil {
			failure(w, 503, "storage_unavailable", "Preferences unavailable")
			return
		}
		respond(w, 200, map[string]string{"language": language, "theme": theme})
	})
	mux.HandleFunc("PUT /api/v1/preferences", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Language string `json:"language"`
			Theme    string `json:"theme"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		user := r.Context().Value(authContextKey{}).(database.User)
		if input.Language != "" {
			if err := store.SetLanguage(r.Context(), user.ID, input.Language); err != nil {
				failure(w, 400, "invalid_language", "Language must be en or de")
				return
			}
		}
		if input.Theme != "" {
			if err := store.SetTheme(r.Context(), user.ID, input.Theme); err != nil {
				failure(w, 400, "invalid_theme", "Theme must be light, dark or auto")
				return
			}
		}
		language, langErr := store.GetLanguage(r.Context(), user.ID)
		if langErr != nil {
			language = "en"
		}
		theme, themeErr := store.GetTheme(r.Context(), user.ID)
		if themeErr != nil {
			theme = "auto"
		}
		respond(w, 200, map[string]string{"language": language, "theme": theme})
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
