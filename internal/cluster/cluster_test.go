package cluster_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	. "github.com/matta813/velora-dns/internal/cluster"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/zones"
)

type testNode struct {
	service *Service
	zones   *zones.Service
	server  *httptest.Server
}

func newNode(t *testing.T, name string, tamper *atomic.Bool) *testNode {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), name+".db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	local, err := zones.New(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(ctx, db, local, Options{Version: "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterPeerRoutes(mux, service)
	handler := http.Handler(mux)
	if tamper != nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !tamper.Load() || !strings.HasSuffix(r.URL.Path, "/snapshot") {
				mux.ServeHTTP(w, r)
				return
			}
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, r)
			for key, values := range recorder.Header() {
				w.Header()[key] = values
			}
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(bytes.Replace(recorder.Body.Bytes(), []byte("192.0.2.10"), []byte("203.0.113.66"), 1))
		})
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &testNode{service: service, zones: local, server: server}
}

func zoneNamed(t *testing.T, z *zones.Service, name string) zones.Zone {
	t.Helper()
	for _, zone := range z.List() {
		if zone.Name == name {
			return zone
		}
	}
	t.Fatalf("zone %s missing", name)
	return zones.Zone{}
}

func TestPrimaryReplicaLifecycle(t *testing.T) {
	ctx := context.Background()
	var tamper atomic.Bool
	primary, replica := newNode(t, "primary", &tamper), newNode(t, "replica", nil)
	if _, err := primary.zones.Create(ctx, zones.Zone{Name: "home.arpa.", PrimaryNS: "ns.home.arpa.", Contact: "hostmaster.home.arpa.", Records: []zones.Record{{Name: "nas.home.arpa.", Type: "A", TTL: 300, Value: "192.0.2.10"}}}); err != nil {
		t.Fatal(err)
	}
	// The replica had its own zone; joining replaces it.
	if _, err := replica.zones.Create(ctx, zones.Zone{Name: "old.arpa.", PrimaryNS: "ns.old.arpa.", Contact: "hostmaster.old.arpa."}); err != nil {
		t.Fatal(err)
	}

	if _, err := primary.service.Create(ctx, "dns1", primary.server.URL, false, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("plain HTTP needs explicit opt-in: %v", err)
	}
	if _, err := primary.service.Create(ctx, "dns1", "http://127.0.0.1:1", true, false); err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("unreachable advertised URL: %v", err)
	}
	if primary.service.Current().Role != RoleStandalone {
		t.Fatal("a failed create must leave the node standalone")
	}
	state, err := primary.service.Create(ctx, "dns1", primary.server.URL+"/", true, false)
	if err != nil || state.Role != RolePrimary || state.AdvertisedURL != primary.server.URL {
		t.Fatalf("create: %+v %v", state, err)
	}
	if _, err = replica.service.Join(ctx, primary.server.URL, "vjt_not-a-real-token", "dns2", true); err == nil || !strings.Contains(err.Error(), "invalid, already used or expired") {
		t.Fatalf("bad token: %v", err)
	}
	token, _, err := primary.service.IssueJoinToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var syncEvents []bool
	replica.service.SetSyncObserver(func(failed bool, _ string) { syncEvents = append(syncEvents, failed) })
	if _, err = replica.service.Join(ctx, primary.server.URL, token, "dns2", true); err != nil {
		t.Fatal(err)
	}
	if !replica.service.ZonesReadOnly() || primary.service.ZonesReadOnly() {
		t.Fatal("only replicas are read-only")
	}
	third := newNode(t, "third", nil)
	if _, err = third.service.Join(ctx, primary.server.URL, token, "dns3", true); err == nil {
		t.Fatal("join tokens are single-use")
	}

	if err = replica.service.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := replica.zones.List(); len(got) != 1 || got[0].Name != "home.arpa." || got[0].Records[0].Value != "192.0.2.10" {
		t.Fatalf("replica zones: %+v", got)
	}
	// The second sync reports the applied revision to the primary.
	if err = replica.service.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	overview, err := primary.service.Overview(ctx)
	if err != nil || len(overview.Members) != 1 || overview.Members[0].Status != "in_sync" || overview.Members[0].Name != "dns2" || overview.Members[0].Version != "1.2.3" {
		t.Fatalf("primary overview: %+v %v", overview, err)
	}

	// Changes on the primary reach the replica; the primary sees it behind first.
	zone := zoneNamed(t, primary.zones, "home.arpa.")
	zone.Records = append(zone.Records, zones.Record{Name: "tv.home.arpa.", Type: "A", TTL: 300, Value: "192.0.2.11"})
	if _, err = primary.zones.Update(ctx, zone.ID, zone.Revision, zone); err != nil {
		t.Fatal(err)
	}
	if _, err = primary.zones.Create(ctx, zones.Zone{Name: "lab.test.", PrimaryNS: "ns.lab.test.", Contact: "hostmaster.lab.test."}); err != nil {
		t.Fatal(err)
	}
	if overview, _ = primary.service.Overview(ctx); overview.Members[0].Status != "behind" {
		t.Fatalf("expected behind: %+v", overview.Members[0])
	}
	if err = replica.service.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(zoneNamed(t, replica.zones, "home.arpa.").Records) != 2 || len(replica.zones.List()) != 2 {
		t.Fatalf("update not applied: %+v", replica.zones.List())
	}
	lab := zoneNamed(t, primary.zones, "lab.test.")
	if err = primary.zones.Delete(ctx, lab.ID, lab.Revision); err != nil {
		t.Fatal(err)
	}

	// A tampered snapshot is rejected and nothing changes.
	tamper.Store(true)
	if err = replica.service.SyncOnce(ctx); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("tampered snapshot: %v", err)
	}
	if len(replica.zones.List()) != 2 {
		t.Fatal("tampered snapshot changed zones")
	}
	tamper.Store(false)
	if err = replica.service.SyncOnce(ctx); err != nil || len(replica.zones.List()) != 1 {
		t.Fatalf("delete not applied: %v %+v", err, replica.zones.List())
	}

	// Removing the replica on the primary stops replication.
	if err = primary.service.RemoveMember(ctx, primary.service.Current().NodeID); !errors.Is(err, ErrConflict) {
		t.Fatalf("primary cannot remove itself: %v", err)
	}
	if err = primary.service.RemoveMember(ctx, replica.service.Current().NodeID); err != nil {
		t.Fatal(err)
	}
	if err = replica.service.SyncOnce(ctx); err == nil || !strings.Contains(err.Error(), "no longer accepts") {
		t.Fatalf("removed replica: %v", err)
	}
	if overview, _ = replica.service.Overview(ctx); !strings.Contains(overview.State.LastSyncError, "no longer accepts") {
		t.Fatalf("replica should show the error: %+v", overview.State)
	}
	// Tampering failed then recovered, removal failed again.
	if len(syncEvents) != 3 || !syncEvents[0] || syncEvents[1] || !syncEvents[2] {
		t.Fatalf("sync events: %v", syncEvents)
	}
	if err = replica.service.Leave(ctx); err != nil || replica.service.ZonesReadOnly() {
		t.Fatalf("leave: %v", err)
	}
	if len(replica.zones.List()) != 1 {
		t.Fatal("leaving keeps the local copy of the zones")
	}
	if err = primary.service.Dissolve(ctx); err != nil || primary.service.Current().Role != RoleStandalone {
		t.Fatalf("dissolve: %v", err)
	}
}

func TestJoinRejectsIncompatibleProtocolAndSelfJoin(t *testing.T) {
	ctx := context.Background()
	primary := newNode(t, "primary", nil)
	if _, err := primary.service.Create(ctx, "dns1", primary.server.URL, true, true); err != nil {
		t.Fatal(err)
	}
	token, _, _ := primary.service.IssueJoinToken(ctx)
	if _, err := primary.service.AcceptJoin(ctx, JoinRequest{Token: token, Protocol: Protocol + 1, NodeID: strings.Repeat("a", 24), Name: "x"}, ""); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("protocol: %v", err)
	}
	if _, err := primary.service.AcceptJoin(ctx, JoinRequest{Token: token, Protocol: Protocol, NodeID: primary.service.Current().NodeID, Name: "x"}, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("self join: %v", err)
	}
	// Peer endpoints reject bad credentials.
	request, _ := http.NewRequest(http.MethodGet, primary.server.URL+"/api/v1/cluster/peer/snapshot", nil)
	request.Header.Set("Authorization", "VeloraNode "+strings.Repeat("b", 24)+":vns_wrong")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || bytes.Contains(body, []byte("zones")) {
		t.Fatalf("unauthenticated snapshot: %d %s", response.StatusCode, body)
	}
}

func TestValidateURL(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		insecure bool
		want     string
	}{
		{"https://dns1.example.lan:8443/", false, "https://dns1.example.lan:8443"},
		{"http://192.168.1.2:8080", true, "http://192.168.1.2:8080"},
		{"http://192.168.1.2:8080", false, ""},
		{"https://user:pw@dns1.example", false, ""},
		{"https://dns1.example/api", false, ""},
		{"ftp://dns1.example", true, ""},
	} {
		got, err := ValidateURL(tc.raw, tc.insecure)
		if (tc.want == "") != (err != nil) || got != tc.want {
			t.Errorf("%s: %q %v", tc.raw, got, err)
		}
	}
}
