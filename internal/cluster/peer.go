package cluster

import (
	"encoding/json"
	"net/http"
)

// BootstrapHandler serves the one public control-plane bootstrap operation.
// It must be mounted only on the pinned-CA TLS listener; the single-use token
// authorizes this endpoint, while all post-join routes require mTLS.
func BootstrapHandler(controller *Controller) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /control/v1/join", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request JoinRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid join request", http.StatusBadRequest)
			return
		}
		response, err := controller.AcceptJoin(r.Context(), request)
		if err != nil {
			http.Error(w, "join rejected", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
	return mux
}
