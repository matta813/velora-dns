package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/zones"
	wire "github.com/miekg/dns"
)

func zoneAPI(t *testing.T) (http.Handler, *zones.Service) {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	c := cache.New(10)
	local, err := zones.New(context.Background(), db, c.Flush)
	if err != nil {
		t.Fatal(err)
	}
	return New(Dependencies{Database: db, DNS: fakeDNS(true), Zones: local, Cache: c, Metrics: metrics.New(c), Config: config.Default(), Started: time.Now()}), local
}
func zoneRequest(h http.Handler, method, path, body, revision string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	if revision != "" {
		r.Header.Set("If-Match", revision)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func decodeZone(t *testing.T, w *httptest.ResponseRecorder, code int) zones.Zone {
	t.Helper()
	if w.Code != code {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data zones.Zone `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}
func TestZoneAndRecordCRUD(t *testing.T) {
	h, local := zoneAPI(t)
	w := zoneRequest(h, "POST", "/api/v1/zones", `{"name":"home.test"}`, "")
	z := decodeZone(t, w, 201)
	path := w.Header().Get("Location")
	if path != fmt.Sprintf("/api/v1/zones/%d", z.ID) || w.Header().Get("ETag") != "\"1\"" {
		t.Fatal("missing location/ETag")
	}
	if w = zoneRequest(h, "GET", "/api/v1/zones", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "home.test.") {
		t.Fatal("zone listing failed")
	}
	w = zoneRequest(h, "POST", path+"/records", `{"name":"host","type":"A","ttl":60,"value":"192.0.2.10"}`, "\"1\"")
	z = decodeZone(t, w, 201)
	recordPath := w.Header().Get("Location")
	recordID := z.Records[0].ID
	if z.Revision != 2 || recordID == 0 {
		t.Fatal("revision/record ID missing")
	}
	for _, route := range []string{path, path + "/records", recordPath} {
		w = zoneRequest(h, "GET", route, "", "")
		if w.Code != 200 || w.Header().Get("ETag") != "\"2\"" {
			t.Fatalf("GET %s: %d", route, w.Code)
		}
	}
	w = zoneRequest(h, "PUT", recordPath, `{"name":"host","type":"A","ttl":60,"value":"192.0.2.11"}`, "\"2\"")
	z = decodeZone(t, w, 200)
	if z.Records[0].ID != recordID || z.Revision != 3 {
		t.Fatal("unstable ID")
	}
	q := new(wire.Msg)
	q.SetQuestion("host.home.test.", wire.TypeA)
	m, ok := local.Lookup(q)
	if !ok || m.Answer[0].(*wire.A).A.String() != "192.0.2.11" {
		t.Fatal("API mutation not activated")
	}
	w = zoneRequest(h, "PUT", path, `{"name":"home.test","primary_ns":"ns.changed.test","contact":"hostmaster.home.test","records":[]}`, "\"3\"")
	z = decodeZone(t, w, 200)
	if z.PrimaryNS != "ns.changed.test." || len(z.Records) != 0 {
		t.Fatal("full replacement failed")
	}
	w = zoneRequest(h, "POST", path+"/records", `{"name":"@","type":"TXT","ttl":0,"value":"literal \"quoted\" text"}`, "\"4\"")
	z = decodeZone(t, w, 201)
	recordPath = w.Header().Get("Location")
	w = zoneRequest(h, "DELETE", recordPath, "", "\"5\"")
	z = decodeZone(t, w, 200)
	if len(z.Records) != 0 || z.Revision != 6 {
		t.Fatal("record delete failed")
	}
	w = zoneRequest(h, "DELETE", path, "", "\"6\"")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = zoneRequest(h, "GET", path, "", ""); w.Code != 404 {
		t.Fatal("zone still exists")
	}
}
func TestZoneAPIValidationAndLostUpdateProtection(t *testing.T) {
	h, _ := zoneAPI(t)
	w := zoneRequest(h, "POST", "/api/v1/zones", `{"name":"home.test"}`, "")
	z := decodeZone(t, w, 201)
	path := fmt.Sprintf("/api/v1/zones/%d", z.ID)
	for _, tc := range []struct {
		method, path, body, etag string
		code                     int
	}{
		{"POST", "/api/v1/zones", `{"name":"HOME.TEST."}`, "", 409},
		{"POST", "/api/v1/zones", `{"name":"other.test","secret":1}`, "", 400},
		{"POST", "/api/v1/zones", `{"name":"other.test"}{}`, "", 400},
		{"POST", "/api/v1/zones", `{"name":"other.test","id":42}`, "", 400},
		{"GET", "/api/v1/zones/abc", "", "", 400},
		{"GET", "/api/v1/zones/999", "", "", 404},
		{"PATCH", path, `{}`, "", 405},
		{"POST", path + "/records", `{"name":"host","type":"A","value":"192.0.2.1"}`, "", 428},
		{"DELETE", path, "", "W/\"1\"", 400},
		{"DELETE", path, "", "\"9\"", 412},
		{"POST", path + "/records", `{"name":"outside.test.","type":"A","value":"192.0.2.1"}`, "\"1\"", 400},
		{"POST", path + "/records", `{"id":100,"name":"host","type":"A","value":"192.0.2.1"}`, "\"1\"", 400},
		{"POST", path + "/records", `{"name":"host","type":"A","value":"invalid"}`, "\"1\"", 400},
	} {
		w = zoneRequest(h, tc.method, tc.path, tc.body, tc.etag)
		if w.Code != tc.code {
			t.Fatalf("%s %s: wanted %d, got %d %s", tc.method, tc.path, tc.code, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"error"`) {
			t.Fatal("inconsistent error envelope")
		}
	}
	w = zoneRequest(h, "POST", path+"/records", `{"name":"host","type":"A","ttl":60,"value":"192.0.2.1"}`, "\"1\"")
	decodeZone(t, w, 201)
	w = zoneRequest(h, "PUT", path, `{"name":"home.test","records":[]}`, "\"1\"")
	if w.Code != 412 {
		t.Fatal("stale replacement accepted")
	}
	fresh := decodeZone(t, zoneRequest(h, "GET", path, "", ""), 200)
	if len(fresh.Records) != 1 {
		t.Fatal("stale write lost a record")
	}
	// The limit also applies to chunked/unknown-length bodies, not only Content-Length.
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/v1/zones", bytes.NewReader([]byte(`{"name":"`+strings.Repeat("a", (1<<20)+1)+`"}`)))
	r.ContentLength = -1
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatalf("chunked payload limit: %d", w.Code)
	}
}
func TestRecordOwnership(t *testing.T) {
	h, _ := zoneAPI(t)
	first := decodeZone(t, zoneRequest(h, "POST", "/api/v1/zones", `{"name":"one.test","records":[{"name":"a","type":"A","value":"192.0.2.1"}]}`, ""), 201)
	second := decodeZone(t, zoneRequest(h, "POST", "/api/v1/zones", `{"name":"two.test"}`, ""), 201)
	path := fmt.Sprintf("/api/v1/zones/%d/records/%d", second.ID, first.Records[0].ID)
	if w := zoneRequest(h, "DELETE", path, "", "\"1\""); w.Code != 404 {
		t.Fatal("foreign record ID accepted")
	}
	body := fmt.Sprintf(`{"name":"two.test","records":[{"id":%d,"name":"a","type":"A","value":"192.0.2.1"}]}`, first.Records[0].ID)
	if w := zoneRequest(h, "PUT", fmt.Sprintf("/api/v1/zones/%d", second.ID), body, "\"1\""); w.Code != 400 {
		t.Fatal("foreign record ID accepted by replacement")
	}
}
