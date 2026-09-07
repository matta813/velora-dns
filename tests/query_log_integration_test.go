package tests

import (
	"context"
	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/database"
	server "github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/filtering"
	"github.com/matta813/velora-dns/internal/querylog"
	"github.com/matta813/velora-dns/internal/zones"
	wire "github.com/miekg/dns"
	"net/netip"
	"path/filepath"
	"testing"
	"time"
)

func TestDNSHistoryCapturesSourcesAndDrains(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	c := cache.New(10)
	local, err := zones.New(ctx, db, c.Flush)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = local.Create(ctx, zones.Zone{Name: "home.test", Records: []zones.Record{{Name: "router", Type: "A", Value: "192.0.2.1", TTL: 60}}}); err != nil {
		t.Fatal(err)
	}
	matcher, err := filtering.New([]filtering.Rule{{Domain: "ads.test", Wildcard: true, Action: filtering.Block}})
	if err != nil {
		t.Fatal(err)
	}
	up, err := server.Start([]string{"127.0.0.1:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
		m := new(wire.Msg)
		m.SetReply(q)
		m.Answer = []wire.RR{&wire.A{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 60}, A: []byte{192, 0, 2, 5}}}
		_ = w.WriteMsg(m)
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = up.Shutdown(ctx) }()
	log := querylog.New(db, true, 16, time.Hour, 100)
	h := &server.Handler{Context: ctx, Resolver: &server.Resolver{Cache: c, Local: local, Filter: matcher, Forwarder: &server.Forwarder{Upstreams: up.Addresses(), Timeout: time.Second}}, Allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Slots: make(chan struct{}, 8), Audit: log}
	s, err := server.Start([]string{"127.0.0.1:0"}, h)
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"external.test.", "external.test.", "router.home.test.", "ads.test."} {
		q := new(wire.Msg)
		q.SetQuestion(name, wire.TypeA)
		transport := "udp"
		if i%2 == 1 {
			transport = "tcp"
		}
		if _, _, err = (&wire.Client{Net: transport, Timeout: time.Second}).Exchange(q, s.Addresses()[0]); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	log.Run(stopped)
	entries, err := db.ListQueries(ctx, querylog.Filter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("drain: %d entries, %+v", len(entries), log.Snapshot())
	}
	sources := map[string]bool{}
	for _, e := range entries {
		sources[e.Source] = true
		if e.ClientIP != "127.0.0.1" || e.Type != "A" || e.Duration <= 0 {
			t.Fatalf("incomplete entry %+v", e)
		}
		if e.Source == "upstream" && e.Upstream != up.Addresses()[0] {
			t.Fatal("upstream missing")
		}
		if e.Source == "cache" && !e.CacheHit {
			t.Fatal("cache hit missing")
		}
	}
	for _, source := range []string{"upstream", "cache", "local", "blocked"} {
		if !sources[source] {
			t.Fatal("source missing: " + source)
		}
	}
}
