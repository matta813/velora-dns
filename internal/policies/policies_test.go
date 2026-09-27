package policies

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/filtering"
)

type memoryStore struct {
	next     int64
	policies map[int64]Policy
}

func (m *memoryStore) LoadPolicies(context.Context) ([]Policy, error) { return nil, nil }
func (m *memoryStore) SavePolicy(_ context.Context, p Policy) (Policy, error) {
	if p.ID == 0 {
		m.next++
		p.ID = m.next
	}
	m.policies[p.ID] = p
	return p, nil
}
func (m *memoryStore) DeletePolicy(_ context.Context, id int64) error {
	delete(m.policies, id)
	return nil
}

type sourceStore struct{ sources []filtering.Source }

func (s *sourceStore) LoadSources(context.Context) ([]filtering.Source, error) { return s.sources, nil }
func (s *sourceStore) SaveSource(_ context.Context, source filtering.Source) (filtering.Source, error) {
	return source, nil
}
func (s *sourceStore) DeleteSource(context.Context, int64) error { return nil }

type fixedClients map[netip.Addr]clients.Client

func (f fixedClients) Lookup(addr netip.Addr) (clients.Client, bool) {
	client, ok := f[addr]
	return client, ok
}

func setup(t *testing.T) (*Service, netip.Addr, netip.Addr, netip.Addr) {
	t.Helper()
	ctx := context.Background()
	global, err := filtering.NewService(ctx, &sourceStore{sources: []filtering.Source{
		{ID: 1, Name: "ads", Enabled: true, Domains: []string{"ads.example"}},
		{ID: 2, Name: "adult", Enabled: false, Domains: []string{"adult.example"}},
	}}, []filtering.Rule{{Domain: "always.example", Wildcard: true, Action: filtering.Block}})
	if err != nil {
		t.Fatal(err)
	}
	kid, tv, guest := netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("192.0.2.20"), netip.MustParseAddr("192.0.2.30")
	known := fixedClients{kid: {ID: 1, Name: "Kid"}, tv: {ID: 2, Name: "TV"}, guest: {ID: 3, Name: "Guest"}}
	service, err := NewService(ctx, &memoryStore{policies: map[int64]Policy{}}, global, known)
	if err != nil {
		t.Fatal(err)
	}
	return service, kid, tv, guest
}

func TestPoliciesPerClient(t *testing.T) {
	ctx := context.Background()
	service, kid, tv, guest := setup(t)
	if _, err := service.Create(ctx, Policy{ClientID: 1, Mode: ModeCustom, Blocklists: []int64{2, 2}, Allow: []string{"ADS.example."}, Block: []string{"games.example"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, Policy{ClientID: 2, Mode: ModeDisabled, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	unknown := netip.MustParseAddr("198.51.100.1")
	for _, tc := range []struct {
		addr    netip.Addr
		name    string
		blocked bool
	}{
		{kid, "adult.example.", true},      // globally disabled list chosen by the policy
		{kid, "cdn.ads.example.", false},   // policy allow beats the global list
		{kid, "games.example.", true},      // extra block
		{kid, "always.example.", true},     // configuration rules still apply
		{tv, "ads.example.", false},        // filtering disabled
		{tv, "always.example.", false},     // including configuration rules
		{guest, "ads.example.", true},      // no policy: global filtering
		{guest, "adult.example.", false},   // global filtering skips disabled lists
		{unknown, "ads.example.", true},    // unknown address: global filtering
		{unknown, "games.example.", false}, // policy extras do not leak
	} {
		if got := service.BlockedFor(tc.addr, tc.name); got != tc.blocked {
			t.Errorf("%s %s: blocked=%v, want %v", tc.addr, tc.name, got, tc.blocked)
		}
	}
	if !service.Blocked("ads.example.") || service.Blocked("games.example.") {
		t.Fatal("Blocked must use the global policy")
	}
	effective := service.Effective(kid)
	if effective.Mode != ModeCustom || effective.Client == nil || effective.Client.Name != "Kid" || effective.Policy == nil {
		t.Fatalf("effective: %+v", effective)
	}
	if got := service.Effective(unknown); got.Mode != ModeDefault || got.Client != nil {
		t.Fatalf("unknown effective: %+v", got)
	}
}

func TestPolicyValidationAndLifecycle(t *testing.T) {
	ctx := context.Background()
	service, kid, _, _ := setup(t)
	for _, bad := range []Policy{
		{ClientID: 0, Mode: ModeDefault},
		{ClientID: 1, Mode: "strict"},
		{ClientID: 1, Mode: ModeCustom, Block: []string{"not a domain"}},
		{ClientID: 1, Mode: ModeCustom, Blocklists: []int64{0}},
	} {
		if _, err := service.Create(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%+v: %v", bad, err)
		}
	}
	created, err := service.Create(ctx, Policy{ClientID: 1, Mode: ModeDisabled, Blocklists: []int64{1}, Enabled: true})
	if err != nil || len(created.Blocklists) != 0 {
		t.Fatalf("%+v %v", created, err)
	}
	if _, err = service.Create(ctx, Policy{ClientID: 1, Mode: ModeDefault}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if service.BlockedFor(kid, "ads.example.") {
		t.Fatal("disabled policy should not block")
	}
	created.Enabled = false
	if _, err = service.Update(ctx, created.ID, created); err != nil {
		t.Fatal(err)
	}
	if !service.BlockedFor(kid, "ads.example.") {
		t.Fatal("a paused policy falls back to global filtering")
	}
	if _, err = service.Update(ctx, 99, created); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	service.ForgetClient(1)
	if list, _ := service.List(ctx); len(list) != 0 {
		t.Fatalf("forget: %+v", list)
	}
	if err = service.Delete(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete: %v", err)
	}
}

func TestRecompilePicksUpBlocklistChanges(t *testing.T) {
	ctx := context.Background()
	store := &sourceStore{sources: []filtering.Source{{ID: 1, Name: "local", Enabled: false, Domains: []string{"old.example"}}}}
	global, err := filtering.NewService(ctx, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	kid := netip.MustParseAddr("192.0.2.10")
	service, err := NewService(ctx, &memoryStore{policies: map[int64]Policy{}}, global, fixedClients{kid: {ID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Create(ctx, Policy{ClientID: 1, Mode: ModeCustom, Blocklists: []int64{1}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = global.ReplaceLocal(ctx, 1, "new.example\n"); err != nil {
		t.Fatal(err)
	}
	service.Recompile()
	if service.BlockedFor(kid, "old.example.") || !service.BlockedFor(kid, "new.example.") {
		t.Fatal("recompile did not use the new list")
	}
}
