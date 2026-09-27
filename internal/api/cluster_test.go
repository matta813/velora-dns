package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/cluster"
	"github.com/matta813/velora-dns/internal/config"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/metrics"
	"github.com/matta813/velora-dns/internal/zones"
)

func TestClusterRoutesRolesPeersAndReplicaZoneLock(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "cluster.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, user := range [][2]string{{"admin", "admin"}, {"operator", "operator"}} {
		if _, err = db.CreateUser(ctx, user[0], user[0]+" password long", user[1]); err != nil {
			t.Fatal(err)
		}
	}
	local, err := zones.New(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	node, err := cluster.NewService(ctx, db, local, cluster.Options{Version: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	memory := cache.New(10)
	h := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: memory, Metrics: metrics.New(memory), Config: config.Default(), Started: time.Now(), Zones: local, ClusterNode: node})
	admin, adminCSRF := loginForTest(t, h, "admin", "admin password long")
	operator, operatorCSRF := loginForTest(t, h, "operator", "operator password long")

	if w := authRequest(h, "GET", "/api/v1/cluster/overview", "", operator, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"role":"standalone"`) {
		t.Fatalf("overview: %d %s", w.Code, w.Body.String())
	}
	create := `{"name":"dns1","advertised_url":"https://dns1.example.lan","skip_check":true}`
	if w := authRequest(h, "POST", "/api/v1/cluster/create", create, operator, operatorCSRF); w.Code != 403 {
		t.Fatalf("operator create: %d", w.Code)
	}
	if w := authRequest(h, "POST", "/api/v1/cluster/create", create, admin, adminCSRF); w.Code != 201 || !strings.Contains(w.Body.String(), `"role":"primary"`) {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := authRequest(h, "POST", "/api/v1/cluster/create", create, admin, adminCSRF); w.Code != 409 {
		t.Fatalf("second create: %d", w.Code)
	}
	w := authRequest(h, "POST", "/api/v1/cluster/join-tokens", "", admin, adminCSRF)
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"token":"vjt_`) {
		t.Fatalf("token: %d %s", w.Code, w.Body.String())
	}

	// Peer routes work without a browser session.
	r := httptest.NewRequest("GET", "http://127.0.0.1/api/v1/cluster/peer/info", nil)
	peer := httptest.NewRecorder()
	h.ServeHTTP(peer, r)
	if peer.Code != 200 || !strings.Contains(peer.Body.String(), `"role":"primary"`) {
		t.Fatalf("peer info: %d %s", peer.Code, peer.Body.String())
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1/api/v1/cluster/peer/snapshot", nil)
	peer = httptest.NewRecorder()
	h.ServeHTTP(peer, r)
	if peer.Code != http.StatusUnauthorized {
		t.Fatalf("snapshot without credential: %d", peer.Code)
	}
	// Non-peer cluster routes still need a session.
	r = httptest.NewRequest("GET", "http://127.0.0.1/api/v1/cluster/overview", nil)
	peer = httptest.NewRecorder()
	h.ServeHTTP(peer, r)
	if peer.Code != http.StatusUnauthorized {
		t.Fatalf("overview without session: %d", peer.Code)
	}

	if w = authRequest(h, "POST", "/api/v1/cluster/dissolve", "", admin, adminCSRF); w.Code != 200 || !strings.Contains(w.Body.String(), `"role":"standalone"`) {
		t.Fatalf("dissolve: %d %s", w.Code, w.Body.String())
	}
	if w = authRequest(h, "POST", "/api/v1/cluster/connect", `{"primary_url":"http://dns1.example.lan","token":"vjt_x","name":"dns2"}`, admin, adminCSRF); w.Code != 400 || !strings.Contains(w.Body.String(), "unencrypted") {
		t.Fatalf("connect over HTTP without opt-in: %d %s", w.Code, w.Body.String())
	}
}

func TestReplicaRejectsZoneWrites(t *testing.T) {
	ctx := context.Background()
	primaryDB, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "primary.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = primaryDB.Close() })
	primaryZones, _ := zones.New(ctx, primaryDB, nil)
	primary, _ := cluster.NewService(ctx, primaryDB, primaryZones, cluster.Options{Version: "1.0.0"})
	peers := http.NewServeMux()
	cluster.RegisterPeerRoutes(peers, primary)
	server := httptest.NewServer(peers)
	defer server.Close()
	if _, err = primary.Create(ctx, "dns1", server.URL, true, true); err != nil {
		t.Fatal(err)
	}
	token, _, _ := primary.IssueJoinToken(ctx)

	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "replica.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.CreateUser(ctx, "admin", "admin password long", "admin"); err != nil {
		t.Fatal(err)
	}
	local, _ := zones.New(ctx, db, nil)
	replica, _ := cluster.NewService(ctx, db, local, cluster.Options{Version: "1.0.0"})
	if _, err = replica.Join(ctx, server.URL, token, "dns2", true); err != nil {
		t.Fatal(err)
	}
	memory := cache.New(10)
	h := New(Dependencies{Database: db, Auth: db, DNS: fakeDNS(true), Cache: memory, Metrics: metrics.New(memory), Config: config.Default(), Started: time.Now(), Zones: local, ClusterNode: replica})
	admin, csrf := loginForTest(t, h, "admin", "admin password long")
	w := authRequest(h, "POST", "/api/v1/zones", `{"name":"x.test","primary_ns":"ns.x.test","contact":"h.x.test","records":[]}`, admin, csrf)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "zones_managed_by_primary") {
		t.Fatalf("replica zone write: %d %s", w.Code, w.Body.String())
	}
	if w = authRequest(h, "GET", "/api/v1/zones", "", admin, ""); w.Code != 200 {
		t.Fatalf("replica zone read: %d", w.Code)
	}
	if w = authRequest(h, "POST", "/api/v1/cluster/sync", "", admin, csrf); w.Code != 200 || !strings.Contains(w.Body.String(), `"role":"replica"`) {
		t.Fatalf("sync now: %d %s", w.Code, w.Body.String())
	}
}
