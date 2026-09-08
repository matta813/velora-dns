package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/auth"
)

const sessionCookie = "velora_session"
const csrfCookie = "velora_csrf"

type principalKey struct{}

func principal(r *http.Request) (auth.User, bool) {
	user, ok := r.Context().Value(principalKey{}).(auth.User)
	return user, ok
}

type loginWindow struct {
	count int
	reset time.Time
}
type loginGuard struct {
	mu      sync.Mutex
	entries map[string]loginWindow
	order   []string
	next    int
	slots   chan struct{}
}

func newLoginGuard() *loginGuard {
	return &loginGuard{entries: map[string]loginWindow{}, slots: make(chan struct{}, 2)}
}
func (g *loginGuard) acquire(ip string, now time.Time) (func(), bool) {
	g.mu.Lock()
	w := g.entries[ip]
	if !now.Before(w.reset) {
		w = loginWindow{reset: now.Add(5 * time.Minute)}
	}
	if w.count >= 5 {
		g.mu.Unlock()
		return nil, false
	}
	w.count++
	if _, exists := g.entries[ip]; !exists {
		if len(g.entries) == 1024 {
			delete(g.entries, g.order[g.next])
			g.order[g.next] = ip
			g.next = (g.next + 1) % 1024
		} else {
			g.order = append(g.order, ip)
		}
	}
	g.entries[ip] = w
	g.mu.Unlock()
	select {
	case g.slots <- struct{}{}:
		return func() { <-g.slots }, true
	default:
		return nil, false
	}
}
func (g *loginGuard) success(ip string) {
	g.mu.Lock()
	window := g.entries[ip]
	window.count = 0
	window.reset = time.Time{}
	g.entries[ip] = window
	g.mu.Unlock()
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func setSessionCookie(w http.ResponseWriter, value string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}
func setCSRFCookie(w http.ResponseWriter, value string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: value, Path: "/", Secure: secure, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}
func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Path: "/", Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

func registerAuth(mux *http.ServeMux, service *auth.Service, secure bool) {
	guard := newLoginGuard()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		ip := remoteIP(r)
		release, ok := guard.acquire(ip, time.Now())
		if !ok {
			failure(w, 429, "login_limited", "Too many login attempts")
			return
		}
		defer release()
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		user, token, csrf, expires, err := service.Login(r.Context(), input.Username, input.Password)
		outcome := "success"
		var actorID *int64
		if err != nil {
			outcome = "failure"
		} else {
			actorID = &user.ID
		}
		auditUsername := input.Username
		if len(auditUsername) > 64 {
			auditUsername = auditUsername[:64]
		}
		if auditErr := service.Store().AddAudit(r.Context(), auth.AuditEvent{OccurredAt: service.Now(), ActorUserID: actorID, ActorUsername: auditUsername, Action: "session.login", Outcome: outcome, RemoteIP: remoteIP(r)}); auditErr != nil {
			if err == nil {
				_ = service.Logout(r.Context(), token)
			}
			failure(w, 503, "audit_unavailable", "Authentication audit storage is unavailable")
			return
		}
		if errors.Is(err, auth.ErrInvalidCredentials) {
			failure(w, 401, "invalid_credentials", "Invalid username or password")
			return
		}
		if err != nil {
			failure(w, 503, "storage_unavailable", "Authentication storage is unavailable")
			return
		}
		guard.success(ip)
		setSessionCookie(w, token, expires, secure)
		setCSRFCookie(w, csrf, expires, secure)
		respond(w, 200, map[string]any{"user": user, "csrf_token": csrf, "expires_at": expires})
	})
	mux.HandleFunc("GET /api/v1/auth/session", func(w http.ResponseWriter, r *http.Request) {
		user, _ := principal(r)
		cookie, _ := r.Cookie(sessionCookie)
		session, err := service.Authenticate(r.Context(), cookie.Value)
		if errors.Is(err, auth.ErrInvalidCredentials) {
			failure(w, 401, "unauthenticated", "Authentication required")
			return
		}
		if err != nil {
			failure(w, 503, "storage_unavailable", "Authentication storage is unavailable")
			return
		}
		respond(w, 200, map[string]any{"user": user, "expires_at": session.ExpiresAt})
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie(sessionCookie)
		user, _ := principal(r)
		if err := service.Logout(r.Context(), cookie.Value); err != nil {
			failure(w, 503, "storage_unavailable", "Session storage is unavailable")
			return
		}
		id := user.ID
		if err := service.Store().AddAudit(r.Context(), auth.AuditEvent{OccurredAt: service.Now(), ActorUserID: &id, ActorUsername: user.Username, Action: "session.logout", Outcome: "success", RemoteIP: remoteIP(r)}); err != nil {
			failure(w, 503, "audit_unavailable", "Audit storage is unavailable")
			return
		}
		clearSessionCookie(w, secure)
		respond(w, 200, map[string]bool{"logged_out": true})
	})
	registerUsers(mux, service)
}

func authenticate(service *auth.Service, secure bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/" || !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			failure(w, 401, "unauthenticated", "Authentication required")
			return
		}
		session, err := service.Authenticate(r.Context(), cookie.Value)
		if errors.Is(err, auth.ErrInvalidCredentials) {
			clearSessionCookie(w, secure)
			failure(w, 401, "unauthenticated", "Authentication required")
			return
		}
		if err != nil {
			failure(w, 503, "storage_unavailable", "Authentication storage is unavailable")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !service.CSRF(session, r.Header.Get("X-CSRF-Token")) {
			failure(w, 403, "invalid_csrf", "A valid CSRF token is required")
			return
		}
		if (strings.HasPrefix(r.URL.Path, "/api/v1/users") || strings.HasPrefix(r.URL.Path, "/api/v1/audit")) && session.User.Role != auth.Admin {
			failure(w, 403, "forbidden", "Administrator role required")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && session.User.Role == auth.Viewer {
			failure(w, 403, "forbidden", "Viewer role is read-only")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
			id := session.User.ID
			if err = service.Store().AddAudit(r.Context(), auth.AuditEvent{OccurredAt: service.Now(), ActorUserID: &id, ActorUsername: session.User.Username, Action: "management." + strings.ToLower(r.Method) + " " + r.URL.Path, Outcome: "success", RemoteIP: remoteIP(r)}); err != nil {
				failure(w, 503, "audit_unavailable", "Audit storage is unavailable")
				return
			}
		}
		ctx := context.WithValue(r.Context(), principalKey{}, session.User)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrNotFound):
		failure(w, 404, "not_found", "User not found")
	case errors.Is(err, auth.ErrConflict):
		failure(w, 409, "user_exists", "Username already exists")
	case errors.Is(err, auth.ErrLastAdmin):
		failure(w, 409, "last_admin", err.Error())
	case errors.Is(err, auth.ErrLimit):
		failure(w, 409, "resource_limit", err.Error())
	default:
		failure(w, 503, "storage_unavailable", "Authentication storage is unavailable")
	}
}
