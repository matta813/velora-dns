package cluster

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
)

// RegisterPeerRoutes adds the node-to-node endpoints. They do not use browser
// sessions: join authenticates with a one-time token, the others with the
// node credential. Management-UI routes live in the api package.
func RegisterPeerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/cluster/peer/info", func(w http.ResponseWriter, r *http.Request) {
		writeData(w, http.StatusOK, s.Info())
	})
	mux.HandleFunc("POST /api/v1/cluster/peer/join", func(w http.ResponseWriter, r *http.Request) {
		var request JoinRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Send a JSON join request")
			return
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		response, err := s.AcceptJoin(r.Context(), request, host)
		if err != nil {
			status, code := peerStatus(err)
			writeError(w, status, code, err.Error())
			return
		}
		writeData(w, http.StatusOK, response)
	})
	mux.HandleFunc("GET /api/v1/cluster/peer/snapshot", func(w http.ResponseWriter, r *http.Request) {
		member, key, err := s.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "node_unauthorized", "This node is not a member of the cluster")
			return
		}
		query := r.URL.Query()
		body, signature, err := s.ServeSnapshot(r.Context(), member, key, query.Get("applied"), query.Get("error"), query.Get("version"))
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "snapshot_unavailable", "The snapshot is unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Velora-Signature", signature)
		_, _ = w.Write(body)
	})
	mux.HandleFunc("POST /api/v1/cluster/peer/leave", func(w http.ResponseWriter, r *http.Request) {
		member, _, err := s.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "node_unauthorized", "This node is not a member of the cluster")
			return
		}
		if err := s.DepartMember(r.Context(), member); err != nil {
			writeError(w, http.StatusServiceUnavailable, "leave_failed", "The node could not be removed")
			return
		}
		writeData(w, http.StatusOK, map[string]string{"left": member.NodeID})
	})
}

func peerStatus(err error) (int, string) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized, "invalid_join_token"
	case errors.Is(err, ErrIncompatible):
		return http.StatusConflict, "incompatible_protocol"
	case errors.Is(err, ErrConflict):
		return http.StatusConflict, "node_conflict"
	case errors.Is(err, ErrState):
		return http.StatusConflict, "not_a_primary"
	case errors.Is(err, ErrInvalid):
		return http.StatusBadRequest, "invalid_request"
	}
	return http.StatusServiceUnavailable, "cluster_unavailable"
}

func writeData(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
