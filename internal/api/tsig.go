package api

import (
	"encoding/json"
	"net/http"

	"github.com/matta813/velora-dns/internal/dns"
)

// TSIGStore is the interface for TSIG key management.
type TSIGStore interface {
	AddKey(name, algorithm, secret string) error
	GetKey(name string) (*dns.TSIGKey, bool)
	RemoveKey(name string) bool
	Keys() []*dns.TSIGKey
}

type tsigKeyResponse struct {
	Name      string `json:"name"`
	Algorithm string `json:"algorithm"`
}

type tsigKeyCreateRequest struct {
	Name      string `json:"name"`
	Algorithm string `json:"algorithm"`
	Secret    string `json:"secret"`
}

func registerTSIG(mux *http.ServeMux, store TSIGStore) {
	mux.HandleFunc("GET /api/v1/tsig-keys", listTSIGKeys(store))
	mux.HandleFunc("POST /api/v1/tsig-keys", createTSIGKey(store))
	mux.HandleFunc("DELETE /api/v1/tsig-keys/{name}", deleteTSIGKey(store))
}

func listTSIGKeys(store TSIGStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keys := store.Keys()
		data := make([]tsigKeyResponse, 0, len(keys))
		for _, k := range keys {
			data = append(data, tsigKeyResponse{Name: k.Name, Algorithm: k.Algorithm})
		}
		respond(w, http.StatusOK, data)
	}
}

func createTSIGKey(store TSIGStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req tsigKeyCreateRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			failure(w, http.StatusBadRequest, "invalid_json", "Invalid JSON")
			return
		}
		if req.Name == "" || req.Algorithm == "" || req.Secret == "" {
			failure(w, http.StatusBadRequest, "missing_fields", "name, algorithm and secret are required")
			return
		}
		if req.Algorithm != "hmac-sha256" && req.Algorithm != "hmac-sha1" && req.Algorithm != "hmac-sha512" {
			failure(w, http.StatusBadRequest, "invalid_algorithm", "algorithm must be hmac-sha256, hmac-sha1, or hmac-sha512")
			return
		}
		if err := store.AddKey(req.Name, req.Algorithm, req.Secret); err != nil {
			failure(w, http.StatusBadRequest, "invalid_key", err.Error())
			return
		}
		respond(w, http.StatusCreated, tsigKeyResponse{Name: req.Name, Algorithm: req.Algorithm})
	}
}

func deleteTSIGKey(store TSIGStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			failure(w, http.StatusBadRequest, "missing_name", "name required")
			return
		}
		if !store.RemoveKey(name) {
			failure(w, http.StatusNotFound, "not_found", "key not found")
			return
		}
		respond(w, http.StatusOK, map[string]string{"name": name})
	}
}
