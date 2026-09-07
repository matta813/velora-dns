package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/matta813/velora-dns/internal/zones"
)

type ZoneStore interface {
	List() []zones.Zone
	Get(int64) (zones.Zone, error)
	Create(context.Context, zones.Zone) (zones.Zone, error)
	Update(context.Context, int64, uint32, zones.Zone) (zones.Zone, error)
	Delete(context.Context, int64, uint32) error
}
type zoneInput struct {
	Name      string         `json:"name"`
	PrimaryNS string         `json:"primary_ns"`
	Contact   string         `json:"contact"`
	Records   []zones.Record `json:"records"`
}

func zoneResponse(w http.ResponseWriter, status int, z zones.Zone) {
	w.Header().Set("ETag", fmt.Sprintf("\"%d\"", z.Revision))
	respond(w, status, z)
}
func zoneID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil || id <= 0 {
		failure(w, 400, "invalid_id", "ID must be a positive integer")
		return 0, false
	}
	return id, true
}
func currentZone(w http.ResponseWriter, r *http.Request, store ZoneStore) (zones.Zone, bool) {
	id, ok := zoneID(w, r, "id")
	if !ok {
		return zones.Zone{}, false
	}
	z, err := store.Get(id)
	if err != nil {
		zoneFailure(w, err)
		return z, false
	}
	return z, true
}
func registerZones(mux *http.ServeMux, store ZoneStore) {
	mux.HandleFunc("GET /api/v1/zones", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, store.List()) })
	mux.HandleFunc("POST /api/v1/zones", func(w http.ResponseWriter, r *http.Request) {
		var input zoneInput
		if !readJSON(w, r, &input) {
			return
		}
		z, err := store.Create(r.Context(), zones.Zone{Name: input.Name, PrimaryNS: input.PrimaryNS, Contact: input.Contact, Records: input.Records})
		if err != nil {
			zoneFailure(w, err)
			return
		}
		w.Header().Set("Location", fmt.Sprintf("/api/v1/zones/%d", z.ID))
		zoneResponse(w, 201, z)
	})
	mux.HandleFunc("GET /api/v1/zones/{id}", func(w http.ResponseWriter, r *http.Request) {
		z, ok := currentZone(w, r, store)
		if ok {
			zoneResponse(w, 200, z)
		}
	})
	mux.HandleFunc("PUT /api/v1/zones/{id}", func(w http.ResponseWriter, r *http.Request) {
		z, ok := currentZone(w, r, store)
		if !ok {
			return
		}
		revision, ok := ifMatch(w, r, z.Revision)
		if !ok {
			return
		}
		var input zoneInput
		if !readJSON(w, r, &input) {
			return
		}
		saved, err := store.Update(r.Context(), z.ID, revision, zones.Zone{Name: input.Name, PrimaryNS: input.PrimaryNS, Contact: input.Contact, Records: input.Records})
		if err != nil {
			zoneFailure(w, err)
			return
		}
		zoneResponse(w, 200, saved)
	})
	mux.HandleFunc("DELETE /api/v1/zones/{id}", func(w http.ResponseWriter, r *http.Request) {
		z, ok := currentZone(w, r, store)
		if !ok {
			return
		}
		revision, ok := ifMatch(w, r, z.Revision)
		if !ok {
			return
		}
		if err := store.Delete(r.Context(), z.ID, revision); err != nil {
			zoneFailure(w, err)
			return
		}
		respond(w, 200, map[string]int64{"deleted": z.ID})
	})
	mux.HandleFunc("/api/v1/zones", methodError("GET, HEAD, POST"))
	mux.HandleFunc("/api/v1/zones/{id}", methodError("GET, HEAD, PUT, DELETE"))
	registerRecords(mux, store)
}
func methodError(allowed string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allowed)
		failure(w, 405, "method_not_allowed", "Method not allowed")
	}
}
