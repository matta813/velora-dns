package forwarding

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/dns"
	wire "github.com/miekg/dns"
)

type memoryStore struct {
	rules []Rule
	next  int64
}

func (m *memoryStore) LoadForwardRules(context.Context) ([]Rule, error) {
	return slices.Clone(m.rules), nil
}
func (m *memoryStore) SaveForwardRule(_ context.Context, rule Rule) (Rule, error) {
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
func (m *memoryStore) DeleteForwardRule(_ context.Context, id int64) error {
	for i := range m.rules {
		if m.rules[i].ID == id {
			m.rules = slices.Delete(m.rules, i, i+1)
			return nil
		}
	}
	return ErrNotFound
}

// resolverAnswering starts a DNS server that answers every A query with ip.
func resolverAnswering(t *testing.T, ip string) string {
	t.Helper()
	server, err := dns.Start([]string{"127.0.0.1:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
		m := new(wire.Msg)
		m.SetReply(q)
		m.Answer = []wire.RR{&wire.A{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 60}, A: net.ParseIP(ip)}}
		_ = w.WriteMsg(m)
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	return server.Addresses()[0]
}

func newService(t *testing.T, options Options) *Service {
	t.Helper()
	if options.Timeout == 0 {
		options.Timeout = 300 * time.Millisecond
	}
	service, err := NewService(context.Background(), &memoryStore{}, options)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func query(t *testing.T, router *Router, name string) (string, error) {
	t.Helper()
	q := new(wire.Msg)
	q.SetQuestion(name, wire.TypeA)
	q.RecursionDesired = true
	m, _, err := router.Resolve(context.Background(), q)
	if err != nil {
		return "", err
	}
	return m.Answer[0].(*wire.A).A.String(), nil
}

func TestParseUpstream(t *testing.T) {
	for raw, want := range map[string]string{"10.0.0.10": "10.0.0.10:53", "10.0.0.10:5353": "10.0.0.10:5353", "::1": "[::1]:53", "[2001:db8::1]:53": "[2001:db8::1]:53", " 192.0.2.1 ": "192.0.2.1:53"} {
		got, err := ParseUpstream(raw)
		if err != nil || got.String() != want {
			t.Errorf("%q: got %v %v, want %s", raw, got, err, want)
		}
	}
	for _, raw := range []string{"dns.example", "0.0.0.0", "224.0.0.1", "10.0.0.1:0", "", "https://1.1.1.1/dns-query"} {
		if _, err := ParseUpstream(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: expected ErrInvalid, got %v", raw, err)
		}
	}
}

func TestRuleValidation(t *testing.T) {
	service := newService(t, Options{Listen: []string{"0.0.0.0:53", "192.0.2.53:5353"}})
	ctx := context.Background()
	for name, rule := range map[string]Rule{
		"wildcard":       {Domain: "*.corp.example", Upstreams: []string{"10.0.0.1"}},
		"bad domain":     {Domain: "corp example", Upstreams: []string{"10.0.0.1"}},
		"no upstreams":   {Domain: "corp.example"},
		"too many":       {Domain: "corp.example", Upstreams: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5"}},
		"duplicate":      {Domain: "corp.example", Upstreams: []string{"10.0.0.1", "10.0.0.1:53"}},
		"loop loopback":  {Domain: "corp.example", Upstreams: []string{"127.0.0.1"}},
		"loop listen ip": {Domain: "corp.example", Upstreams: []string{"192.0.2.53:5353"}},
		"multiline":      {Domain: "corp.example", Upstreams: []string{"10.0.0.1"}, Description: "a\nb"},
	} {
		if _, err := service.Create(ctx, rule); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: expected ErrInvalid, got %v", name, err)
		}
	}
	rule, err := service.Create(ctx, Rule{Domain: "Corp.Example.", Upstreams: []string{"10.0.0.1"}, Enabled: true})
	if err != nil || rule.Domain != "corp.example" || rule.Upstreams[0] != "10.0.0.1:53" {
		t.Fatalf("normalized create: %+v %v", rule, err)
	}
	if _, err = service.Create(ctx, Rule{Domain: "corp.example", Upstreams: []string{"10.0.0.2"}}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate domain: %v", err)
	}
	if _, err = service.Update(ctx, 99, Rule{Domain: "x.example", Upstreams: []string{"10.0.0.2"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
}

func TestMostSpecificRuleWinsAndOthersUseDefault(t *testing.T) {
	global, corp, lab := resolverAnswering(t, "192.0.2.1"), resolverAnswering(t, "192.0.2.2"), resolverAnswering(t, "192.0.2.3")
	var changed []string
	service := newService(t, Options{OnChange: func(domain string) { changed = append(changed, domain) }})
	ctx := context.Background()
	if _, err := service.Create(ctx, Rule{Domain: "corp.example", Upstreams: []string{corp}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	labRule, err := service.Create(ctx, Rule{Domain: "lab.corp.example", Upstreams: []string{lab}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	router := &Router{Rules: service, Default: &dns.Forwarder{Upstreams: []string{global}, Timeout: time.Second}}
	for name, want := range map[string]string{
		"corp.example.":        "192.0.2.2",
		"host.corp.example.":   "192.0.2.2",
		"db.LAB.corp.example.": "192.0.2.3",
		"lab.corp.example.":    "192.0.2.3",
		"notcorp.example.":     "192.0.2.1",
		"corp.example.org.":    "192.0.2.1",
		"example.":             "192.0.2.1",
	} {
		if got, err := query(t, router, name); err != nil || got != want {
			t.Errorf("%s: got %s %v, want %s", name, got, err, want)
		}
	}
	labRule.Enabled = false
	if _, err = service.Update(ctx, labRule.ID, labRule); err != nil {
		t.Fatal(err)
	}
	if got, _ := query(t, router, "db.lab.corp.example."); got != "192.0.2.2" {
		t.Fatalf("disabled rule should fall back to parent rule, got %s", got)
	}
	if err = service.Delete(ctx, labRule.ID); err != nil {
		t.Fatal(err)
	}
	if want := []string{"corp.example", "lab.corp.example", "lab.corp.example", "lab.corp.example"}; !slices.Equal(changed, want) {
		t.Fatalf("cache invalidation callbacks %v, want %v", changed, want)
	}
}

func TestBrokenRuleOnlyAffectsItsDomain(t *testing.T) {
	global := resolverAnswering(t, "192.0.2.1")
	closed, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddress := closed.LocalAddr().String()
	_ = closed.Close()
	service := newService(t, Options{Timeout: 100 * time.Millisecond})
	if _, err = service.Create(context.Background(), Rule{Domain: "broken.example", Upstreams: []string{deadAddress}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	router := &Router{Rules: service, Default: &dns.Forwarder{Upstreams: []string{global}, Timeout: time.Second}}
	if _, err = query(t, router, "host.broken.example."); err == nil {
		t.Fatal("broken rule must not silently fall back to global upstreams")
	}
	if got, err := query(t, router, "fine.example."); err != nil || got != "192.0.2.1" {
		t.Fatalf("unrelated names must keep resolving: %s %v", got, err)
	}
	rules, _ := service.List(context.Background())
	if len(rules) != 1 || len(rules[0].Health) != 1 || rules[0].Health[0].ConsecutiveFailures == 0 {
		t.Fatalf("rule health not reported: %+v", rules)
	}
}

func TestRuleTestAction(t *testing.T) {
	corp := resolverAnswering(t, "192.0.2.2")
	service := newService(t, Options{})
	rule, err := service.Create(context.Background(), Rule{Domain: "corp.example", Upstreams: []string{corp}, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Test(context.Background(), rule.ID, "", 0)
	if err != nil || result.Rcode != "NOERROR" || result.Upstream != corp || len(result.Answers) != 1 || result.Name != "corp.example." {
		t.Fatalf("test result: %+v %v", result, err)
	}
	if _, err = service.Test(context.Background(), rule.ID, "other.example", 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("names outside the rule must be rejected: %v", err)
	}
	if _, err = service.Test(context.Background(), 42, "", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing rule: %v", err)
	}
}
