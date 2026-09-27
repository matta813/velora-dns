package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/querylog"
)

func TestClientsEndpointsAndQueryNames(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	service, err := clients.NewService(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err = db.WriteQueries(ctx, []querylog.Entry{
		{OccurredAt: now.Add(-time.Minute), ClientIP: "192.168.1.200", Domain: "tv.test.", Type: "A", Rcode: "NOERROR", Source: "cache"},
		{OccurredAt: now.Add(-time.Minute), ClientIP: "192.168.1.9", Domain: "x.test.", Type: "A", Rcode: "NOERROR", Source: "cache"},
	}, now.Add(-time.Hour), 100); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerClients(mux, service, db, func() bool { return true })
	registerQueries(mux, db, func() bool { return true }, service.Name)

	if w := zoneRequest(mux, "POST", "/api/v1/clients", `{"name":"Living Room TV","addresses":["192.168.1.200"],"group":"Media"}`, ""); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := zoneRequest(mux, "POST", "/api/v1/clients", `{"name":"Dup","addresses":["192.168.1.200/32"]}`, ""); w.Code != 409 {
		t.Fatalf("overlap: %d", w.Code)
	}
	if w := zoneRequest(mux, "POST", "/api/v1/clients", `{"name":"Bad","addresses":["tv.local"]}`, ""); w.Code != 400 {
		t.Fatalf("invalid: %d", w.Code)
	}
	w := zoneRequest(mux, "GET", "/api/v1/clients", "", "")
	var list struct {
		Data struct {
			Clients []struct {
				Name     string
				Activity *struct{ Queries uint64 }
			}
			Unnamed []struct {
				ClientIP string `json:"client_ip"`
			}
		}
	}
	if err = json.Unmarshal(w.Body.Bytes(), &list); err != nil || w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if len(list.Data.Clients) != 1 || list.Data.Clients[0].Activity == nil || list.Data.Clients[0].Activity.Queries != 1 || len(list.Data.Unnamed) != 1 || list.Data.Unnamed[0].ClientIP != "192.168.1.9" {
		t.Fatalf("activity: %s", w.Body.String())
	}
	w = zoneRequest(mux, "GET", "/api/v1/queries?client=192.168.1.200", "", "")
	var queries struct{ Data []querylog.Entry }
	if err = json.Unmarshal(w.Body.Bytes(), &queries); err != nil || len(queries.Data) != 1 || queries.Data[0].ClientName != "Living Room TV" {
		t.Fatalf("query names: %s", w.Body.String())
	}
	w = zoneRequest(mux, "GET", "/api/v1/query-stats?window=1h", "", "")
	var stats struct{ Data querylog.Summary }
	if err = json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	named := map[string]string{}
	for _, item := range stats.Data.TopClients {
		named[item.Value] = item.Name
	}
	if named["192.168.1.200"] != "Living Room TV" || named["192.168.1.9"] != "" {
		t.Fatalf("ranking names: %s", w.Body.String())
	}
}
