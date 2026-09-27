package api

import (
	"context"
	"net/http"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/filtering"
	"github.com/matta813/velora-dns/internal/policies"
)

func TestPolicyEndpoints(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	known, err := clients.NewService(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	lists, err := filtering.NewService(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	source, err := lists.Create(ctx, "Kids", "", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lists.ReplaceLocal(ctx, source.ID, "games.example\n"); err != nil {
		t.Fatal(err)
	}
	kid, err := known.Create(ctx, clients.Client{Name: "Kid", Addresses: []string{"192.168.1.40"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	service, err := policies.NewService(ctx, db, lists, known)
	if err != nil {
		t.Fatal(err)
	}
	known.SetDeleteObserver(service.ForgetClient)
	mux := http.NewServeMux()
	registerPolicies(mux, service, known, lists)
	registerClients(mux, known, nil, func() bool { return false })

	for _, bad := range []struct {
		body string
		code int
	}{
		{`{"client_id":99,"mode":"disabled"}`, 400},
		{`{"client_id":1,"mode":"custom","blocklists":[42]}`, 400},
		{`{"client_id":1,"mode":"strict"}`, 400},
		{`{"client_id":1,"mode":"custom","unknown":true}`, 400},
	} {
		if w := zoneRequest(mux, "POST", "/api/v1/policies", bad.body, ""); w.Code != bad.code {
			t.Fatalf("%s: %d %s", bad.body, w.Code, w.Body.String())
		}
	}
	body := `{"client_id":` + strconv.FormatInt(kid.ID, 10) + `,"mode":"custom","blocklists":[` + strconv.FormatInt(source.ID, 10) + `],"block":["video.example"]}`
	w := zoneRequest(mux, "POST", "/api/v1/policies", body, "")
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w = zoneRequest(mux, "POST", "/api/v1/policies", body, ""); w.Code != 409 {
		t.Fatalf("duplicate: %d", w.Code)
	}
	if !service.BlockedFor(netip.MustParseAddr("192.168.1.40"), "games.example.") || service.Blocked("games.example.") {
		t.Fatal("custom policy should use the globally disabled list")
	}
	w = zoneRequest(mux, "GET", "/api/v1/policies/effective?ip=192.168.1.40", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mode":"custom"`) || !strings.Contains(w.Body.String(), `"name":"Kid"`) {
		t.Fatalf("effective: %d %s", w.Code, w.Body.String())
	}
	if w = zoneRequest(mux, "GET", "/api/v1/policies/effective?ip=nope", "", ""); w.Code != 400 {
		t.Fatalf("effective invalid: %d", w.Code)
	}
	if w = zoneRequest(mux, "PUT", "/api/v1/policies/1", `{"client_id":`+strconv.FormatInt(kid.ID, 10)+`,"mode":"disabled","enabled":false}`, ""); w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	if w = zoneRequest(mux, "PUT", "/api/v1/policies/9", `{"client_id":`+strconv.FormatInt(kid.ID, 10)+`,"mode":"disabled"}`, ""); w.Code != 404 {
		t.Fatalf("update missing: %d", w.Code)
	}
	if w = zoneRequest(mux, "DELETE", "/api/v1/clients/"+strconv.FormatInt(kid.ID, 10), "", ""); w.Code != 200 {
		t.Fatalf("delete client: %d", w.Code)
	}
	if w = zoneRequest(mux, "GET", "/api/v1/policies", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatalf("policy should go with its client: %s", w.Body.String())
	}
}
