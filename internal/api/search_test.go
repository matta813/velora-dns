package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/zones"
)

type fixedZones []zones.Zone

func (z fixedZones) List() []zones.Zone                                     { return z }
func (z fixedZones) Get(int64) (zones.Zone, error)                          { return zones.Zone{}, nil }
func (z fixedZones) Create(context.Context, zones.Zone) (zones.Zone, error) { return zones.Zone{}, nil }
func (z fixedZones) Update(context.Context, int64, uint32, zones.Zone) (zones.Zone, error) {
	return zones.Zone{}, nil
}
func (z fixedZones) Delete(context.Context, int64, uint32) error { return nil }

func TestSearchFindsResourcesAndStaysBounded(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "search.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	known, err := clients.NewService(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = known.Create(ctx, clients.Client{Name: "Living Room TV", Addresses: []string{"192.168.1.20"}, Group: "Media", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	records := make([]zones.Record, 0, 40)
	for i := 0; i < 40; i++ {
		records = append(records, zones.Record{ID: int64(i), Name: "host.home.arpa", Type: "A", Value: "192.168.1.1"})
	}
	zoneList := fixedZones{{ID: 7, Name: "home.arpa", PrimaryNS: "ns.home.arpa", Records: records}}
	mux := http.NewServeMux()
	registerSearch(mux, searchSources{zones: zoneList, clients: known})

	var body struct{ Data []SearchResult }
	w := zoneRequest(mux, "GET", "/api/v1/search?q=HOME", "", "")
	if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	kinds := map[string]int{}
	for _, result := range body.Data {
		kinds[result.Kind]++
	}
	if kinds["zone"] != 1 || kinds["record"] != searchPerKind || body.Data[0].Link != "/zones?zone=7" {
		t.Fatalf("results: %+v", body.Data)
	}
	w = zoneRequest(mux, "GET", "/api/v1/search?q=media", "", "")
	if !strings.Contains(w.Body.String(), `"link":"/clients?name=Living+Room+TV"`) {
		t.Fatalf("client by group: %s", w.Body.String())
	}
	w = zoneRequest(mux, "GET", "/api/v1/search?q=192.168.1.20", "", "")
	if !strings.Contains(w.Body.String(), `"kind":"client"`) {
		t.Fatalf("client by address: %s", w.Body.String())
	}
	for _, bad := range []string{"", "a", strings.Repeat("x", 101)} {
		if got := zoneRequest(mux, "GET", "/api/v1/search?q="+bad, "", ""); got.Code != 400 {
			t.Fatalf("%q: %d", bad, got.Code)
		}
	}
	if got := zoneRequest(mux, "GET", "/api/v1/search?q=nothing-matches", "", ""); got.Body.String() != "{\"data\":[]}\n" {
		t.Fatalf("empty: %s", got.Body.String())
	}
}
