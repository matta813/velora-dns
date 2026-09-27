package rewrites

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/filtering"
	"github.com/matta813/velora-dns/internal/zones"
	wire "github.com/miekg/dns"
)

type memoryStore struct {
	rules []Rule
	next  int64
}

func (m *memoryStore) LoadRewrites(context.Context) ([]Rule, error) {
	return slices.Clone(m.rules), nil
}
func (m *memoryStore) SaveRewrite(_ context.Context, rule Rule) (Rule, error) {
	if rule.ID == 0 {
		m.next++
		rule.ID = m.next
		m.rules = append(m.rules, rule)
		return rule, nil
	}
	for i := range m.rules {
		if m.rules[i].ID == rule.ID {
			m.rules[i] = rule
			return rule, nil
		}
	}
	return rule, ErrNotFound
}
func (m *memoryStore) DeleteRewrite(_ context.Context, id int64) error {
	for i := range m.rules {
		if m.rules[i].ID == id {
			m.rules = slices.Delete(m.rules, i, i+1)
			return nil
		}
	}
	return ErrNotFound
}

func service(t *testing.T, hints Hints, rules ...Rule) *Service {
	t.Helper()
	s, err := NewService(context.Background(), &memoryStore{}, hints)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range rules {
		rule.Enabled = true
		if _, err = s.Create(context.Background(), rule); err != nil {
			t.Fatalf("create %+v: %v", rule, err)
		}
	}
	return s
}

func ask(t *testing.T, s *Service, name string, qtype uint16) (*wire.Msg, bool) {
	t.Helper()
	q := new(wire.Msg)
	q.SetQuestion(name, qtype)
	return s.Rewrite(q)
}

func TestValidation(t *testing.T) {
	s := service(t, Hints{})
	for label, rule := range map[string]Rule{
		"bad name":        {Name: "nas home", Type: "A", Value: "192.0.2.1"},
		"inner wildcard":  {Name: "a.*.home", Type: "A", Value: "192.0.2.1"},
		"tld wildcard":    {Name: "*.home", Type: "A", Value: "192.0.2.1"},
		"v6 as A":         {Name: "nas.home", Type: "A", Value: "2001:db8::1"},
		"v4 as AAAA":      {Name: "nas.home", Type: "AAAA", Value: "192.0.2.1"},
		"bad type":        {Name: "nas.home", Type: "MX", Value: "mail.home"},
		"self CNAME":      {Name: "nas.home", Type: "CNAME", Value: "nas.home."},
		"wildcard target": {Name: "nas.home", Type: "CNAME", Value: "*.home"},
		"multiline note":  {Name: "nas.home", Type: "A", Value: "192.0.2.1", Description: "a\nb"},
	} {
		if _, err := s.Create(context.Background(), rule); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: expected ErrInvalid, got %v", label, err)
		}
	}
	rule, err := s.Create(context.Background(), Rule{Name: " NAS.Home. ", Type: "a", Value: "192.0.2.5", Enabled: true})
	if err != nil || rule.Name != "nas.home" || rule.Type != "A" {
		t.Fatalf("normalize: %+v %v", rule, err)
	}
	if _, err = s.Create(context.Background(), Rule{Name: "nas.home", Type: "A", Value: "192.0.2.5"}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err = s.Create(context.Background(), Rule{Name: "nas.home", Type: "CNAME", Value: "other.home"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("CNAME beside A: %v", err)
	}
	if _, err = s.Create(context.Background(), Rule{Name: "alias.home", Type: "CNAME", Value: "nas.home"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(context.Background(), Rule{Name: "alias.home", Type: "A", Value: "192.0.2.9"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("A beside CNAME: %v", err)
	}
}

func TestMatchingPrecedence(t *testing.T) {
	s := service(t, Hints{},
		Rule{Name: "*.lab.home", Type: "A", Value: "192.0.2.10"},
		Rule{Name: "*.dev.lab.home", Type: "A", Value: "192.0.2.20"},
		Rule{Name: "db.dev.lab.home", Type: "A", Value: "192.0.2.30"},
		Rule{Name: "db.dev.lab.home", Type: "A", Value: "192.0.2.31"},
		Rule{Name: "db.dev.lab.home", Type: "AAAA", Value: "2001:db8::30"},
	)
	for name, want := range map[string][]string{
		"x.lab.home.":       {"192.0.2.10"},
		"a.b.lab.home.":     {"192.0.2.10"},
		"api.dev.lab.home.": {"192.0.2.20"},
		"DB.dev.lab.home.":  {"192.0.2.30", "192.0.2.31"},
	} {
		m, ok := ask(t, s, name, wire.TypeA)
		var got []string
		if ok {
			for _, rr := range m.Answer {
				got = append(got, rr.(*wire.A).A.String())
				if rr.Header().Name != name {
					t.Errorf("%s: owner %s must equal the question", name, rr.Header().Name)
				}
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
	if _, ok := ask(t, s, "lab.home.", wire.TypeA); ok {
		t.Error("a wildcard must not match its parent domain")
	}
	if _, ok := ask(t, s, "other.home.", wire.TypeA); ok {
		t.Error("unrelated name rewritten")
	}
	m, ok := ask(t, s, "db.dev.lab.home.", wire.TypeAAAA)
	if !ok || len(m.Answer) != 1 || m.Answer[0].(*wire.AAAA).AAAA.String() != "2001:db8::30" {
		t.Fatalf("AAAA rewrite: %v", m)
	}
	m, ok = ask(t, s, "x.lab.home.", wire.TypeAAAA)
	if !ok || m.Rcode != wire.RcodeSuccess || len(m.Answer) != 0 {
		t.Fatalf("rewritten names must answer NODATA for other types, got %v %v", ok, m)
	}
}

func TestDisabledAndDeletedRulesStopAnswering(t *testing.T) {
	var changed []string
	s := service(t, Hints{OnChange: func(name string) { changed = append(changed, name) }}, Rule{Name: "*.lab.home", Type: "A", Value: "192.0.2.10"})
	rule := s.rules[0]
	rule.Enabled = false
	if _, err := s.Update(context.Background(), rule.ID, rule); err != nil {
		t.Fatal(err)
	}
	if _, ok := ask(t, s, "x.lab.home.", wire.TypeA); ok {
		t.Fatal("disabled rule still answers")
	}
	if err := s.Delete(context.Background(), rule.ID); err != nil {
		t.Fatal(err)
	}
	if want := []string{"lab.home", "lab.home", "lab.home"}; !slices.Equal(changed, want) {
		t.Fatalf("invalidation names %v, want %v (wildcards report their parent)", changed, want)
	}
}

type zoneStore struct{ zones []zones.Zone }

func (z *zoneStore) LoadZones(context.Context) ([]zones.Zone, error) { return z.zones, nil }
func (z *zoneStore) SaveZone(_ context.Context, zone zones.Zone, _ uint32) (zones.Zone, error) {
	return zone, nil
}
func (z *zoneStore) DeleteZone(context.Context, int64, uint32) error { return nil }

type sourceStore struct{ sources []filtering.Source }

func (s *sourceStore) LoadSources(context.Context) ([]filtering.Source, error) { return s.sources, nil }
func (s *sourceStore) SaveSource(_ context.Context, source filtering.Source) (filtering.Source, error) {
	return source, nil
}
func (s *sourceStore) DeleteSource(context.Context, int64) error { return nil }

type upstream map[string]string

func (u upstream) Resolve(_ context.Context, q *wire.Msg) (*wire.Msg, string, error) {
	m := new(wire.Msg)
	m.SetReply(q)
	if ip, ok := u[q.Question[0].Name]; ok {
		rr, _ := wire.NewRR(q.Question[0].Name + " 60 IN A " + ip)
		m.Answer = []wire.RR{rr}
	}
	return m, "test-upstream", nil
}

func TestResolverPrecedence(t *testing.T) {
	ctx := context.Background()
	local, err := zones.New(ctx, &zoneStore{zones: []zones.Zone{{ID: 1, Name: "home", PrimaryNS: "ns.home", Contact: "admin.home", Records: []zones.Record{{Name: "nas.home.", Type: "A", TTL: 60, Value: "192.0.2.100"}, {Name: "printer.home.", Type: "A", TTL: 60, Value: "192.0.2.101"}}}}}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	filter, err := filtering.NewService(ctx, &sourceStore{}, []filtering.Rule{{Domain: "ads.home", Wildcard: true, Action: filtering.Block}})
	if err != nil {
		t.Fatal(err)
	}
	s := service(t, Hints{Blocked: filter.Blocked, ZoneFor: local.ZoneFor},
		Rule{Name: "nas.home", Type: "A", Value: "192.0.2.5"},
		Rule{Name: "ads.home", Type: "A", Value: "192.0.2.6"},
		Rule{Name: "docs.home", Type: "CNAME", Value: "docs.example.org"},
	)
	resolver := dns.NewResolver(&Local{Rewrites: s, Next: local}, filter, cache.New(10), upstream{"docs.example.org.": "198.51.100.7"}, "NXDOMAIN")
	answer := func(name string) (dns.Result, string) {
		q := new(wire.Msg)
		q.SetQuestion(name, wire.TypeA)
		q.RecursionDesired = true
		result, err := resolver.Resolve(ctx, q)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		last := ""
		if n := len(result.Message.Answer); n > 0 {
			if a, ok := result.Message.Answer[n-1].(*wire.A); ok {
				last = a.A.String()
			}
		}
		return result, last
	}
	if _, ip := answer("nas.home."); ip != "192.0.2.5" {
		t.Errorf("rewrite must override the local zone, got %s", ip)
	}
	if _, ip := answer("printer.home."); ip != "192.0.2.101" {
		t.Errorf("zone answers must remain for other names, got %s", ip)
	}
	if result, _ := answer("ads.home."); result.Source != "blocked" {
		t.Errorf("blocklists must win over rewrites, got %s", result.Source)
	}
	if result, ip := answer("docs.home."); ip != "198.51.100.7" || len(result.Message.Answer) != 2 {
		t.Errorf("CNAME rewrite must be followed: %v", result.Message)
	}
	status, _ := s.List(ctx)
	hints := map[string]RuleStatus{}
	for _, rule := range status {
		hints[rule.Name] = rule
	}
	if hints["ads.home"].BlockedBy != "blocklist" || hints["nas.home"].OverridesZone != "home" || hints["docs.home"].BlockedBy != "" {
		t.Fatalf("precedence hints: %+v", status)
	}
}
