package api

import (
	"encoding/json"
	"github.com/matta813/velora-dns/internal/filtering"
	"net/http"
	"strconv"
	"strings"
)

func registerBlocklists(mux *http.ServeMux, service *filtering.Service) {
	mux.HandleFunc("GET /api/v1/blocklists", func(w http.ResponseWriter, r *http.Request) {
		sources, err := service.List(r.Context())
		if err != nil {
			failure(w, 503, "storage_unavailable", "Blocklist storage is unavailable")
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
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.URL) == "" {
			failure(w, 400, "invalid_request", "A source name and JSON body are required")
			return
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		source, err := service.Create(r.Context(), strings.TrimSpace(input.Name), strings.TrimSpace(input.URL), enabled)
		if err != nil {
			failure(w, 400, "invalid_source", "Source could not be created")
			return
		}
		respond(w, 201, source)
	})
	mux.HandleFunc("POST /api/v1/blocklists/{id}/update", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			failure(w, 400, "invalid_id", "Invalid source ID")
			return
		}
		source, err := service.Refresh(r.Context(), id)
		if err != nil {
			failure(w, 502, "update_failed", "Source update failed; prior rules remain active")
			return
		}
		respond(w, 200, source)
	})
}
