package api

import (
	"context"
	"net/http"
)

type OnboardingStore interface {
	UserCount(ctx context.Context) (int, error)
}

func registerOnboarding(mux *http.ServeMux, store OnboardingStore, config any) {
	mux.HandleFunc("GET /api/v1/onboarding/status", func(w http.ResponseWriter, r *http.Request) {
		userCount, err := store.UserCount(r.Context())
		if err != nil {
			failure(w, 503, "database_error", "Could not check onboarding status")
			return
		}

		isFirstRun := userCount == 0

		respond(w, 200, map[string]any{
			"first_run":    isFirstRun,
			"user_count":   userCount,
			"config_ready": true,
		})
	})
}
