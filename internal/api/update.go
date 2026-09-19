package api

import (
	"net/http"
	"time"

	"github.com/matta813/velora-dns/internal/update"
)

type UpdateStore interface {
	Current() *update.Entry
	History() []update.Entry
	IsUpdating() bool
}

func registerUpdate(mux *http.ServeMux, store UpdateStore, version Version) {
	mux.HandleFunc("GET /api/v1/update/status", func(w http.ResponseWriter, r *http.Request) {
		current := store.Current()
		if current == nil {
			history := store.History()
			lastState := "idle"
			var lastCompleted time.Time
			if len(history) > 0 {
				last := history[len(history)-1]
				lastState = string(last.State)
				lastCompleted = last.CompletedAt
			}
			respond(w, 200, map[string]any{
				"state":          lastState,
				"installed":      version.Version,
				"last_completed": lastCompleted,
				"updating":       false,
			})
			return
		}
		respond(w, 200, map[string]any{
			"state":        string(current.State),
			"installed":    version.Version,
			"from_version": current.FromVersion,
			"to_version":   current.ToVersion,
			"started_at":   current.StartedAt,
			"updating":     true,
		})
	})

	mux.HandleFunc("GET /api/v1/update/history", func(w http.ResponseWriter, r *http.Request) {
		history := store.History()
		respond(w, 200, history)
	})

	mux.HandleFunc("POST /api/v1/update/request", func(w http.ResponseWriter, r *http.Request) {
		if store.IsUpdating() {
			failure(w, 409, "update_in_progress", "An update is already in progress")
			return
		}

		type updateRequest struct {
			Action string `json:"action"`
		}
		var req updateRequest
		if !readJSON(w, r, &req) {
			return
		}
		if req.Action != "update" {
			failure(w, 400, "invalid_action", "Only 'update' action is supported")
			return
		}

		respond(w, 202, map[string]string{
			"status":  "accepted",
			"message": "Update request accepted. Check status for progress.",
		})
	})
}
