package dns_test

import (
	"context"
	"net"
	"net/netip"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/clients"
	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/filtering"
	"github.com/matta813/velora-dns/internal/forwarding"
	"github.com/matta813/velora-dns/internal/policies"
	"github.com/matta813/velora-dns/internal/rewrites"
	"github.com/matta813/velora-dns/internal/zones"
	wire "github.com/miekg/dns"
)

// countingUpstream answers every A query and counts how often it is asked.
type countingUpstream struct{ calls atomic.Int32 }

func (u *countingUpstream) Resolve(_ context.Context, q *wire.Msg) (*wire.Msg, string, error) {
	u.calls.Add(1)
	m := new(wire.Msg)
	m.SetReply(q)
	rr, _ := wire.NewRR(q.Question[0].Name + " 120 IN A 198.51.100.9")
	m.Answer = []wire.RR{rr}
	return m, "global", nil
}

type explainFixture struct {
	resolver *dns.Resolver
	cache    *cache.Cache
	global   *countingUpstream
	rule     *atomic.Int32 // queries received by the corp.example rule upstream
}

func newExplainFixture(t *testing.T) explainFixture {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "explain.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	local, err := zones.New(ctx, db, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = local.Create(ctx, zones.Zone{Name: "home", PrimaryNS: "ns.home", Contact: "admin.home", Records: []zones.Record{
		{Name: "nas.home.", Type: "A", TTL: 60, Value: "192.0.2.100"},
		{Name: "off.home.", Type: "A", TTL: 60, Value: "192.0.2.101"},
	}}); err != nil {
		t.Fatal(err)
	}

	lists, err := filtering.NewService(ctx, db, []filtering.Rule{
		{Domain: "static.example", Wildcard: true, Action: filtering.Block},
		{Domain: "ok.ads.example", Action: filtering.Allow},
	})
	if err != nil {
		t.Fatal(err)
	}
	ads, err := lists.Create(ctx, "Ads", "", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lists.ReplaceLocal(ctx, ads.ID, "ads.example\n"); err != nil {
		t.Fatal(err)
	}
	kids, err := lists.Create(ctx, "Kids", "", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lists.ReplaceLocal(ctx, kids.ID, "games.example\n"); err != nil {
		t.Fatal(err)
	}

	known, err := clients.NewService(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := known.Create(ctx, clients.Client{Name: "Dev", Addresses: []string{"192.168.1.10"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	kid, err := known.Create(ctx, clients.Client{Name: "Kid", Addresses: []string{"192.168.1.40"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.NewService(ctx, db, lists, known)
	if err != nil {
		t.Fatal(err)
	}
	lists.SetChangeObserver(policy.Recompile)
	if _, err = policy.Create(ctx, policies.Policy{ClientID: dev.ID, Mode: policies.ModeDisabled, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = policy.Create(ctx, policies.Policy{ClientID: kid.ID, Mode: policies.ModeCustom, Blocklists: []int64{kids.ID}, Block: []string{"extra.example"}, Allow: []string{"ok.kid.example"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	rules, err := rewrites.NewService(ctx, db, rewrites.Hints{Blocked: lists.Blocked, ZoneFor: local.ZoneFor})
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []rewrites.Rule{
		{Name: "nas.home", Type: "A", Value: "192.0.2.5", Enabled: true},
		{Name: "*.lab.home", Type: "A", Value: "192.0.2.10", Enabled: true},
		{Name: "off.home", Type: "A", Value: "192.0.2.77", Enabled: false},
		{Name: "cn.rewrite.example", Type: "CNAME", Value: "target.corp.example", Enabled: true},
		{Name: "evil.rewrite.example", Type: "CNAME", Value: "ads.example", Enabled: true},
	} {
		if _, err = rules.Create(ctx, rule); err != nil {
			t.Fatalf("rewrite %s: %v", rule.Name, err)
		}
	}

	var ruleQueries atomic.Int32
	fake, err := dns.Start([]string{"127.0.0.1:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
		ruleQueries.Add(1)
		m := new(wire.Msg)
		m.SetReply(q)
		m.Answer = []wire.RR{&wire.A{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 60}, A: net.ParseIP("203.0.113.5")}}
		_ = w.WriteMsg(m)
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = fake.Shutdown(shutdown)
	})
	routes, err := forwarding.NewService(ctx, db, forwarding.Options{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = routes.Create(ctx, forwarding.Rule{Domain: "corp.example", Upstreams: fake.Addresses()[:1], Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = routes.Create(ctx, forwarding.Rule{Domain: "lab2.example", Upstreams: fake.Addresses()[:1], Enabled: false}); err != nil {
		t.Fatal(err)
	}

	memory := cache.New(100)
	put := func(name string, records ...string) {
		q := new(wire.Msg)
		q.SetQuestion(name, wire.TypeA)
		q.RecursionDesired = true
		m := new(wire.Msg)
		m.SetReply(q)
		for _, text := range records {
			rr, err := wire.NewRR(text)
			if err != nil {
				t.Fatal(err)
			}
			m.Answer = append(m.Answer, rr)
		}
		memory.Put(q, m)
	}
	put("cached.example.", "cached.example. 120 IN A 198.51.100.1")
	put("cdn.example.", "cdn.example. 120 IN CNAME ads.example.", "ads.example. 120 IN A 198.51.100.2")

	global := &countingUpstream{}
	return explainFixture{
		resolver: dns.NewResolver(&rewrites.Local{Rewrites: rules, Next: local}, policy, memory, &forwarding.Router{Rules: routes, Default: global}, "NXDOMAIN"),
		cache:    memory, global: global, rule: &ruleQueries,
	}
}

func TestExplainPrecedenceMatchesRealResolution(t *testing.T) {
	type want struct {
		source, stage, result string
		rule                  string // winner rule name, "" = none expected
		ruleID                int64
		depth                 int
	}
	cases := []struct {
		label, name string
		qtype       uint16
		client      string
		want        want
	}{
		{"rewrite beats zone", "nas.home.", wire.TypeA, "", want{"local", "rewrite", "answered", "nas.home", 1, 0}},
		{"rewrite ignores client without policy", "nas.home.", wire.TypeA, "192.168.9.9", want{"local", "rewrite", "answered", "nas.home", 1, 0}},
		{"rewrite NODATA for other type", "nas.home.", wire.TypeAAAA, "", want{"local", "rewrite", "answered", "nas.home", 1, 0}},
		{"zone answers when no rewrite", "other.home.", wire.TypeA, "", want{"local", "zone", "answered", "home", 0, 0}},
		{"disabled rewrite is ignored", "off.home.", wire.TypeA, "", want{"local", "zone", "answered", "home", 0, 0}},
		{"wildcard rewrite", "x.lab.home.", wire.TypeA, "", want{"local", "rewrite", "answered", "*.lab.home", 2, 0}},
		{"blocklist blocks before rewrite or upstream", "ads.example.", wire.TypeA, "", want{"blocked", "filter", "blocked", "Ads", 1, 0}},
		{"configuration rule blocks", "static.example.", wire.TypeA, "", want{"blocked", "filter", "blocked", "filtering.blocklist configuration rule", 0, 0}},
		{"policy disabled bypasses blocklist", "ads.example.", wire.TypeA, "192.168.1.10", want{"upstream", "upstream", "would_forward", "", 0, 0}},
		{"allow rule beats blocklist", "ok.ads.example.", wire.TypeA, "", want{"upstream", "upstream", "would_forward", "", 0, 0}},
		{"custom list is not global", "games.example.", wire.TypeA, "", want{"upstream", "upstream", "would_forward", "", 0, 0}},
		{"custom list blocks its client", "games.example.", wire.TypeA, "192.168.1.40", want{"blocked", "filter", "blocked", "Kids", 2, 0}},
		{"policy domain blocks", "extra.example.", wire.TypeA, "192.168.1.40", want{"blocked", "filter", "blocked", "client policy domains", 0, 0}},
		{"policy domain only applies to its client", "extra.example.", wire.TypeA, "", want{"upstream", "upstream", "would_forward", "", 0, 0}},
		{"custom policy ignores global lists", "ads.example.", wire.TypeA, "192.168.1.40", want{"upstream", "upstream", "would_forward", "", 0, 0}},
		{"conditional forwarding rule", "app.corp.example.", wire.TypeA, "", want{"upstream", "forwarding", "would_forward", "corp.example", 1, 0}},
		{"disabled forwarding rule falls back", "x.lab2.example.", wire.TypeA, "", want{"upstream", "upstream", "would_forward", "", 0, 0}},
		{"cache hit", "cached.example.", wire.TypeA, "", want{"cache", "cache", "hit", "", 0, 0}},
		{"cached blocked CNAME target", "cdn.example.", wire.TypeA, "", want{"blocked", "filter", "blocked", "Ads", 1, 0}},
		{"cached answer for client with bypass", "cdn.example.", wire.TypeA, "192.168.1.10", want{"cache", "cache", "hit", "", 0, 0}},
		{"rewrite CNAME continues to forwarding rule", "cn.rewrite.example.", wire.TypeA, "", want{"local", "rewrite", "answered", "cn.rewrite.example", 4, 0}},
		{"rewrite CNAME to blocked target", "evil.rewrite.example.", wire.TypeA, "", want{"blocked", "filter", "blocked", "Ads", 1, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			f := newExplainFixture(t) // fresh cache: earlier real queries must not leak in
			var client netip.Addr
			if tc.client != "" {
				client = netip.MustParseAddr(tc.client)
			}
			upstreamBefore, ruleBefore, cacheBefore := f.global.calls.Load(), f.rule.Load(), f.cache.Stats()
			got := f.resolver.Explain(context.Background(), tc.name, tc.qtype, client)
			if f.global.calls.Load() != upstreamBefore || f.rule.Load() != ruleBefore {
				t.Fatal("explain contacted an upstream")
			}
			if f.cache.Stats() != cacheBefore {
				t.Fatalf("explain changed the cache: %+v -> %+v", cacheBefore, f.cache.Stats())
			}
			if got.Source != tc.want.source || got.Winner.Stage != tc.want.stage || got.Winner.Result != tc.want.result || got.Winner.Depth != tc.want.depth {
				t.Fatalf("source %q winner %+v, want %+v\nsteps: %+v", got.Source, got.Winner, tc.want, got.Steps)
			}
			var names []string
			var id int64
			for _, r := range got.Winner.Rules {
				names, id = append(names, r.Name), r.ID
			}
			if f := got.Winner.Filter; f != nil {
				for _, m := range f.Matches {
					names, id = append(names, m.Name), m.ID
				}
			}
			if tc.want.rule != "" && (!slices.Contains(names, tc.want.rule) || tc.want.ruleID != 0 && id != tc.want.ruleID) {
				t.Fatalf("winner rules %v (id %d), want %q (id %d)", names, id, tc.want.rule, tc.want.ruleID)
			}
			// The same code decides real queries: source and rcode must agree.
			q := new(wire.Msg)
			q.SetQuestion(tc.name, tc.qtype)
			ctx := context.Background()
			if client.IsValid() {
				ctx = dns.WithClient(ctx, client)
			}
			real, err := f.resolver.Resolve(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			if real.Source != got.Source {
				t.Fatalf("real query used %q, explain says %q", real.Source, got.Source)
			}
			if got.Source != "upstream" && wire.RcodeToString[real.Message.Rcode] != got.Rcode {
				t.Fatalf("rcode real %s explain %s", wire.RcodeToString[real.Message.Rcode], got.Rcode)
			}
			if got.Source == "local" && tc.want.rule != "cn.rewrite.example" {
				var answers []string
				for _, rr := range real.Message.Answer {
					answers = append(answers, rr.String())
				}
				if !slices.Equal(answers, got.Answers) {
					t.Fatalf("answers real %v explain %v", answers, got.Answers)
				}
			}
		})
	}
}

func TestExplainDetails(t *testing.T) {
	f := newExplainFixture(t)
	explain := func(name string, client string) dns.Explanation {
		var addr netip.Addr
		if client != "" {
			addr = netip.MustParseAddr(client)
		}
		return f.resolver.Explain(context.Background(), name, wire.TypeA, addr)
	}

	t.Run("cached answer reports remaining TTL without touching counters", func(t *testing.T) {
		before := f.cache.Stats()
		got := explain("cached.example.", "")
		for i := 0; i < 3; i++ {
			explain("cached.example.", "")
		}
		if got.Winner.TTL == nil || *got.Winner.TTL > 120 || *got.Winner.TTL < 110 {
			t.Fatalf("remaining TTL %v", got.Winner.TTL)
		}
		if f.cache.Stats() != before {
			t.Fatalf("cache counters changed: %+v -> %+v", before, f.cache.Stats())
		}
	})
	t.Run("steps list stages in resolver order", func(t *testing.T) {
		got := explain("app.corp.example.", "")
		var stages []string
		for _, s := range got.Steps {
			stages = append(stages, s.Stage+":"+s.Result)
		}
		want := []string{"filter:passed", "rewrite:no_match", "zone:no_match", "cache:miss", "forwarding:would_forward"}
		if !slices.Equal(stages, want) {
			t.Fatalf("steps %v, want %v", stages, want)
		}
		if got.Winner.Rules[0].ID == 0 || got.Winner.Rules[0].Name != "corp.example" {
			t.Fatalf("forwarding rule %+v", got.Winner.Rules)
		}
	})
	t.Run("filter step names client policy and allow rules", func(t *testing.T) {
		got := explain("ok.kid.example.", "192.168.1.40")
		info := got.Steps[0].Filter
		if got.Steps[0].Result != "allowed" || info == nil || info.Scope != "client_policy" || info.Client != "Kid" || len(info.Allowed) != 1 {
			t.Fatalf("filter step %+v %+v", got.Steps[0], info)
		}
		got = explain("ok.ads.example.", "")
		if got.Steps[0].Result != "allowed" || got.Steps[0].Filter.Scope != "global" {
			t.Fatalf("global allow %+v", got.Steps[0])
		}
	})
	t.Run("blocked by a policy-disabled client lists no matches", func(t *testing.T) {
		got := explain("ads.example.", "192.168.1.10")
		if info := got.Steps[0].Filter; got.Steps[0].Result != "passed" || info.Mode != policies.ModeDisabled || len(info.Matches) != 0 {
			t.Fatalf("%+v %+v", got.Steps[0], info)
		}
	})
	t.Run("no filter means no filter step", func(t *testing.T) {
		r := dns.NewResolver(nil, nil, cache.New(1), f.global, "NXDOMAIN")
		got := r.Explain(context.Background(), "a.example", wire.TypeA, netip.Addr{})
		if len(got.Steps) != 2 || got.Steps[0].Stage != "cache" || got.Winner.Stage != "upstream" {
			t.Fatalf("%+v", got.Steps)
		}
	})
	if f.global.calls.Load() != 0 || f.rule.Load() != 0 {
		t.Fatal("explain contacted an upstream")
	}
}

func TestParseType(t *testing.T) {
	for _, name := range []string{"A", "aaaa", "Mx", "TXT", "PTR", "NS", "SOA", "CNAME"} {
		if _, ok := dns.ParseType(name); !ok {
			t.Errorf("%s should be accepted", name)
		}
	}
	for _, name := range []string{"", "ANY", "AXFR", "DNSKEY", "BOGUS", "65280"} {
		if _, ok := dns.ParseType(name); ok {
			t.Errorf("%q should be rejected", name)
		}
	}
}
