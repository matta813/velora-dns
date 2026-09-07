package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/matta813/velora-dns/internal/zones"
)

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		failure(w, 415, "unsupported_media_type", "Use application/json")
		return false
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err = d.Decode(target)
	if err == nil {
		var extra any
		err = d.Decode(&extra)
		if err == io.EOF {
			return true
		}
		if err == nil {
			err = errors.New("multiple values")
		}
	}
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		failure(w, 413, "payload_too_large", "Request exceeds 1 MiB")
	} else {
		failure(w, 400, "invalid_json", "Expected one JSON object with known fields and valid field types")
	}
	return false
}
func ifMatch(w http.ResponseWriter, r *http.Request, current uint32) (uint32, bool) {
	value := strings.TrimSpace(r.Header.Get("If-Match"))
	if value == "" {
		failure(w, 428, "precondition_required", "Send If-Match with the current quoted zone revision")
		return 0, false
	}
	if len(r.Header.Values("If-Match")) != 1 || len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		failure(w, 400, "invalid_precondition", "If-Match requires one strong quoted revision")
		return 0, false
	}
	n, err := strconv.ParseUint(value[1:len(value)-1], 10, 32)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != value[1:len(value)-1] {
		failure(w, 400, "invalid_precondition", "Invalid zone revision")
		return 0, false
	}
	if uint32(n) != current {
		failure(w, 412, "revision_conflict", "Zone changed; reload before retrying")
		return 0, false
	}
	return uint32(n), true
}
func zoneFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, zones.ErrNotFound):
		failure(w, 404, "not_found", "Zone or record not found")
	case errors.Is(err, zones.ErrConflict):
		failure(w, 412, "revision_conflict", "Zone changed; reload before retrying")
	case errors.Is(err, zones.ErrExists):
		failure(w, 409, "zone_exists", "A zone with this name already exists")
	case errors.Is(err, zones.ErrInvalid):
		failure(w, 400, "invalid_zone", err.Error())
	default:
		failure(w, 503, "storage_unavailable", "Zone storage is unavailable; no change was activated")
	}
}
