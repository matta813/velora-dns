package api

import (
	"context"
	"errors"
	"github.com/matta813/velora-dns/internal/filtering"
	"net/http"
	"strconv"
	"time"
)

type BlocklistStore interface {
	List(context.Context) ([]filtering.Source, error)
	Create(context.Context, string, string, bool) (filtering.Source, error)
	Refresh(context.Context, int64) (filtering.Source, error)
	ReplaceLocal(context.Context, int64, string) (filtering.Source, error)
	SetEnabled(context.Context, int64, bool) (filtering.Source, error)
	Delete(context.Context, int64) error
}

func blocklistError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, filtering.ErrNotFound):
		failure(w, 404, "not_found", err.Error())
	case errors.Is(err, filtering.ErrInvalid):
		failure(w, 400, "invalid_source", err.Error())
	case errors.Is(err, filtering.ErrExists):
		failure(w, 409, "source_exists", err.Error())
	case errors.Is(err, filtering.ErrBusy):
		failure(w, 409, "busy", err.Error())
	default:
		failure(w, 502, "update_failed", "Source operation failed; previous DNS rules remain active")
	}
}
func sourceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		failure(w, 400, "invalid_id", "Invalid source ID")
		return 0, false
	}
	return id, true
}
func registerBlocklists(mux *http.ServeMux, service BlocklistStore) {
	mux.HandleFunc("GET /api/v1/blocklists", func(w http.ResponseWriter, r *http.Request) {
		sources, err := service.List(r.Context())
		if err != nil {
			blocklistError(w, err)
			return
		}
		respond(w, 200, sources)
	})
	mux.HandleFunc("POST /api/v1/blocklists", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Enabled *bool  `json:"enabled"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		source, err := service.Create(r.Context(), input.Name, input.URL, enabled)
		if err != nil {
			blocklistError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/blocklists/"+strconv.FormatInt(source.ID, 10))
		respond(w, 201, source)
	})
	mux.HandleFunc("PUT /api/v1/blocklists/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := sourceID(w, r)
		if !ok {
			return
		}
		var input struct {
			Enabled *bool `json:"enabled"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		if input.Enabled == nil {
			failure(w, 400, "invalid_request", "enabled is required")
			return
		}
		source, err := service.SetEnabled(r.Context(), id, *input.Enabled)
		if err != nil {
			blocklistError(w, err)
			return
		}
		respond(w, 200, source)
	})
	mux.HandleFunc("PUT /api/v1/blocklists/{id}/content", func(w http.ResponseWriter, r *http.Request) {
		id, ok := sourceID(w, r)
		if !ok {
			return
		}
		var input struct {
			Content string `json:"content"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		source, err := service.ReplaceLocal(r.Context(), id, input.Content)
		if err != nil {
			blocklistError(w, err)
			return
		}
		respond(w, 200, source)
	})
	refresh := func(w http.ResponseWriter, r *http.Request, id int64) {
		ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
		defer cancel()
		source, err := service.Refresh(ctx, id)
		if err != nil {
			blocklistError(w, err)
			return
		}
		respond(w, 200, source)
	}
	mux.HandleFunc("POST /api/v1/blocklists/{id}/update", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := sourceID(w, r); ok {
			refresh(w, r, id)
		}
	})
	mux.HandleFunc("DELETE /api/v1/blocklists/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := sourceID(w, r)
		if !ok {
			return
		}
		if err := service.Delete(r.Context(), id); err != nil {
			blocklistError(w, err)
			return
		}
		respond(w, 200, map[string]int64{"deleted": id})
	})
	for path, methods := range map[string]string{"/api/v1/blocklists": "GET, HEAD, POST", "/api/v1/blocklists/{id}": "PUT, DELETE", "/api/v1/blocklists/{id}/content": "PUT", "/api/v1/blocklists/{id}/update": "POST"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", methods)
			failure(w, 405, "method_not_allowed", "Method not allowed")
		})
	}
}
