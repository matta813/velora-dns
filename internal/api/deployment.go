package api

import (
	"encoding/json"
	"net/http"

	"github.com/matta813/velora-dns/internal/deployment"
)

func registerDeployment(mux *http.ServeMux, agent Deployment) {
	mux.HandleFunc("GET /api/v1/deployment/settings", func(w http.ResponseWriter, r *http.Request) {
		result, err := agent.Get(r.Context())
		if err != nil {
			failure(w, http.StatusServiceUnavailable, "deployment_agent_unavailable", "Local deployment agent is unavailable")
			return
		}
		respond(w, http.StatusOK, result)
	})
	mux.HandleFunc("PUT /api/v1/deployment/settings", func(w http.ResponseWriter, r *http.Request) {
		var settings deployment.Settings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil || settings.Validate() != nil {
			failure(w, http.StatusBadRequest, "invalid_settings", "Choose a supported release channel and exposure")
			return
		}
		result, err := agent.Apply(r.Context(), settings)
		if err != nil {
			failure(w, http.StatusServiceUnavailable, "deployment_agent_unavailable", "Local deployment agent could not apply settings")
			return
		}
		respond(w, http.StatusOK, result)
	})
}
