package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/matta813/velora-dns/internal/zones"
)

type secondaryZoneRequest struct {
	Name             string `json:"name"`
	PrimaryAddress   string `json:"primary_address"`
	TransferTSIGKey  string `json:"transfer_tsig_key"`
	TransferInterval int    `json:"transfer_interval"`
}

type secondaryZoneResponse struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	ZoneType           string  `json:"zone_type"`
	PrimaryAddress     string  `json:"primary_address"`
	TransferTSIGKey    string  `json:"transfer_tsig_key"`
	TransferInterval   int     `json:"transfer_interval"`
	LastTransferSerial uint32  `json:"last_transfer_serial"`
	LastTransferAt     *string `json:"last_transfer_at,omitempty"`
	NextRefreshAt      *string `json:"next_refresh_at,omitempty"`
}

func registerSecondaryZones(mux *http.ServeMux, store ZoneStore) {
	mux.HandleFunc("POST /api/v1/zones/secondary", createSecondaryZone(store))
	mux.HandleFunc("GET /api/v1/zones/{id}/transfer-status", getTransferStatus(store))
}

func createSecondaryZone(store ZoneStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req secondaryZoneRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			failure(w, http.StatusBadRequest, "invalid_json", "Invalid JSON")
			return
		}
		if req.Name == "" || req.PrimaryAddress == "" {
			failure(w, http.StatusBadRequest, "missing_fields", "name and primary_address are required")
			return
		}
		if req.TransferInterval <= 0 {
			req.TransferInterval = 3600
		}
		z := zones.Zone{
			Name:             req.Name,
			ZoneType:         "secondary",
			PrimaryAddress:   req.PrimaryAddress,
			TransferTSIGKey:  req.TransferTSIGKey,
			TransferInterval: req.TransferInterval,
		}
		saved, err := store.Create(r.Context(), z)
		if err != nil {
			failure(w, http.StatusBadRequest, "creation_failed", err.Error())
			return
		}
		resp := secondaryZoneResponse{
			ID:               saved.ID,
			Name:             saved.Name,
			ZoneType:         saved.ZoneType,
			PrimaryAddress:   saved.PrimaryAddress,
			TransferTSIGKey:  saved.TransferTSIGKey,
			TransferInterval: saved.TransferInterval,
		}
		respond(w, http.StatusCreated, resp)
	}
}

func getTransferStatus(store ZoneStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			failure(w, http.StatusBadRequest, "invalid_id", "invalid zone id")
			return
		}
		z, err := store.Get(id)
		if err != nil {
			failure(w, http.StatusNotFound, "not_found", "zone not found")
			return
		}
		resp := secondaryZoneResponse{
			ID:                 z.ID,
			Name:               z.Name,
			ZoneType:           z.ZoneType,
			PrimaryAddress:     z.PrimaryAddress,
			TransferTSIGKey:    z.TransferTSIGKey,
			TransferInterval:   z.TransferInterval,
			LastTransferSerial: z.LastTransferSerial,
		}
		if z.LastTransferAt != nil {
			s := z.LastTransferAt.Format("2006-01-02T15:04:05Z")
			resp.LastTransferAt = &s
		}
		if z.NextRefreshAt != nil {
			s := z.NextRefreshAt.Format("2006-01-02T15:04:05Z")
			resp.NextRefreshAt = &s
		}
		respond(w, http.StatusOK, resp)
	}
}
