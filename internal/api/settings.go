package api

import (
	"context"
	"net/http"

	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/dns"
)

type SettingsStore interface {
	GetRateLimitSettings(context.Context) (database.RateLimitSettings, error)
	SetRateLimitSettings(context.Context, database.RateLimitSettings) error
}

func registerSettings(mux *http.ServeMux, store SettingsStore, state *dns.RateLimitState) {
	mux.HandleFunc("GET /api/v1/settings/rate-limit", func(w http.ResponseWriter, r *http.Request) {
		settings, err := store.GetRateLimitSettings(r.Context())
		if err != nil {
			failure(w, 503, "storage_unavailable", "Rate limit settings unavailable")
			return
		}
		respond(w, 200, settings)
	})
	mux.HandleFunc("PUT /api/v1/settings/rate-limit", func(w http.ResponseWriter, r *http.Request) {
		var input database.RateLimitSettings
		if !readJSON(w, r, &input) {
			return
		}
		if err := store.SetRateLimitSettings(r.Context(), input); err != nil {
			failure(w, 400, "invalid_settings", "Rate limit values are invalid")
			return
		}
		state.Configure(input.Enabled, input.GlobalQPS, input.ClientQPS, input.RateLimitBurst)
		respond(w, 200, input)
	})
}
