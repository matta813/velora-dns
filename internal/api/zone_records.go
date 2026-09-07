package api

import (
	"fmt"
	"net/http"

	"github.com/matta813/velora-dns/internal/zones"
)

func recordIndex(w http.ResponseWriter, r *http.Request, z zones.Zone) (int, bool) {
	id, ok := zoneID(w, r, "recordID")
	if !ok {
		return 0, false
	}
	for i, record := range z.Records {
		if record.ID == id {
			return i, true
		}
	}
	zoneFailure(w, zones.ErrNotFound)
	return 0, false
}
func registerRecords(mux *http.ServeMux, store ZoneStore) {
	mux.HandleFunc("GET /api/v1/zones/{id}/records", func(w http.ResponseWriter, r *http.Request) {
		z, ok := currentZone(w, r, store)
		if !ok {
			return
		}
		w.Header().Set("ETag", fmt.Sprintf("\"%d\"", z.Revision))
		respond(w, 200, z.Records)
	})
	mux.HandleFunc("GET /api/v1/zones/{id}/records/{recordID}", func(w http.ResponseWriter, r *http.Request) {
		z, ok := currentZone(w, r, store)
		if !ok {
			return
		}
		i, ok := recordIndex(w, r, z)
		if !ok {
			return
		}
		w.Header().Set("ETag", fmt.Sprintf("\"%d\"", z.Revision))
		respond(w, 200, z.Records[i])
	})
	mux.HandleFunc("POST /api/v1/zones/{id}/records", func(w http.ResponseWriter, r *http.Request) {
		z, ok := currentZone(w, r, store)
		if !ok {
			return
		}
		revision, ok := ifMatch(w, r, z.Revision)
		if !ok {
			return
		}
		var record zones.Record
		if !readJSON(w, r, &record) {
			return
		}
		if record.ID != 0 {
			failure(w, 400, "invalid_record", "Record IDs are assigned by the server")
			return
		}
		z.Records = append(z.Records, record)
		saved, err := store.Update(r.Context(), z.ID, revision, z)
		if err != nil {
			zoneFailure(w, err)
			return
		}
		w.Header().Set("Location", fmt.Sprintf("/api/v1/zones/%d/records/%d", saved.ID, saved.Records[len(saved.Records)-1].ID))
		zoneResponse(w, 201, saved)
	})
	mux.HandleFunc("PUT /api/v1/zones/{id}/records/{recordID}", func(w http.ResponseWriter, r *http.Request) {
		z, ok := currentZone(w, r, store)
		if !ok {
			return
		}
		i, ok := recordIndex(w, r, z)
		if !ok {
			return
		}
		revision, ok := ifMatch(w, r, z.Revision)
		if !ok {
			return
		}
		var record zones.Record
		if !readJSON(w, r, &record) {
			return
		}
		if record.ID != 0 {
			failure(w, 400, "invalid_record", "Record ID is selected by the URL")
			return
		}
		record.ID = z.Records[i].ID
		z.Records[i] = record
		saved, err := store.Update(r.Context(), z.ID, revision, z)
		if err != nil {
			zoneFailure(w, err)
			return
		}
		zoneResponse(w, 200, saved)
	})
	mux.HandleFunc("DELETE /api/v1/zones/{id}/records/{recordID}", func(w http.ResponseWriter, r *http.Request) {
		z, ok := currentZone(w, r, store)
		if !ok {
			return
		}
		i, ok := recordIndex(w, r, z)
		if !ok {
			return
		}
		revision, ok := ifMatch(w, r, z.Revision)
		if !ok {
			return
		}
		z.Records = append(z.Records[:i], z.Records[i+1:]...)
		saved, err := store.Update(r.Context(), z.ID, revision, z)
		if err != nil {
			zoneFailure(w, err)
			return
		}
		zoneResponse(w, 200, saved)
	})
	mux.HandleFunc("/api/v1/zones/{id}/records", methodError("GET, HEAD, POST"))
	mux.HandleFunc("/api/v1/zones/{id}/records/{recordID}", methodError("GET, HEAD, PUT, DELETE"))
}
