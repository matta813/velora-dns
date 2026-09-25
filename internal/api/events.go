package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/matta813/velora-dns/internal/database"
)

type EventStore interface {
	ListSystemEvents(context.Context, int64, string, int) (database.SystemEventPage, error)
	MarkSystemEventRead(context.Context, int64, string, int64) error
}

func registerEvents(mux *http.ServeMux, store EventStore) {
	mux.HandleFunc("GET /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(authContextKey{}).(database.User)
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 200 {
				failure(w, 400, "invalid_limit", "Limit must be between 1 and 200")
				return
			}
			limit = value
		}
		page, err := store.ListSystemEvents(r.Context(), user.ID, user.Role, limit)
		if err != nil {
			failure(w, 503, "events_unavailable", "System events unavailable")
			return
		}
		respond(w, 200, page)
	})
	mux.HandleFunc("POST /api/v1/events/{id}/read", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			failure(w, 415, "unsupported_media_type", "Use application/json")
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Invalid event ID")
			return
		}
		user := r.Context().Value(authContextKey{}).(database.User)
		if err := store.MarkSystemEventRead(r.Context(), user.ID, user.Role, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				failure(w, 404, "not_found", "Event not found")
				return
			}
			failure(w, 503, "events_unavailable", "Could not mark event as read")
			return
		}
		respond(w, 200, map[string]bool{"read": true})
	})
}
