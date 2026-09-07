package tests

import (
	"context"
	"net/netip"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	"github.com/matta813/velora-dns/internal/database"
	server "github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/zones"
	wire "github.com/miekg/dns"
)

func TestLocalZoneOverridesCacheAndNeverForwardsMisses(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "zones.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	c := cache.New(10)
	local, err := zones.New(ctx, db, c.Flush)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	up, err := server.Start([]string{"127.0.0.1:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
		calls.Add(1)
		m := new(wire.Msg)
		m.SetReply(q)
		rr, e := wire.NewRR(q.Question[0].Name + " 60 IN A 192.0.2.99")
		if e != nil {
			t.Error(e)
			return
		}
		m.Answer = []wire.RR{rr}
		_ = w.WriteMsg(m)
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = up.Shutdown(ctx) }()
	h := &server.Handler{Context: ctx, Resolver: &server.Resolver{Cache: c, Local: local, Forwarder: &server.Forwarder{Upstreams: up.Addresses(), Timeout: time.Second}}, Allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Slots: make(chan struct{}, 8)}
	s, err := server.Start([]string{"127.0.0.1:0"}, h)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Shutdown(ctx) }()
	query := func(name, transport string, kind uint16, recursive bool) *wire.Msg {
		t.Helper()
		q := new(wire.Msg)
		q.SetQuestion(name, kind)
		q.RecursionDesired = recursive
		m, _, err := (&wire.Client{Net: transport, Timeout: time.Second}).Exchange(q, s.Addresses()[0])
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	query("host.home.test.", "udp", wire.TypeA, true)
	z, err := local.Create(ctx, zones.Zone{Name: "home.test", Records: []zones.Record{{Name: "host", Type: "A", TTL: 60, Value: "192.0.2.1"}, {Name: "external", Type: "CNAME", TTL: 60, Value: "external.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, transport := range []string{"udp", "tcp"} {
		m := query("host.home.test.", transport, wire.TypeA, false)
		if !m.Authoritative || len(m.Answer) != 1 || m.Answer[0].(*wire.A).A.String() != "192.0.2.1" {
			t.Fatalf("local precedence failed: %v", m)
		}
		m = query("missing.home.test.", transport, wire.TypeA, true)
		if m.Rcode != wire.RcodeNameError || !m.Authoritative || len(m.Ns) != 1 {
			t.Fatalf("local negative response: %v", m)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("local miss leaked upstream")
	}
	m := query("external.home.test.", "tcp", wire.TypeA, true)
	if len(m.Answer) != 2 || m.Answer[1].(*wire.A).A.String() != "192.0.2.99" {
		t.Fatalf("external alias not completed: %v", m)
	}
	z.Records[0].Value = "192.0.2.2"
	z, err = local.Update(ctx, z.ID, z.Revision, z)
	if err != nil {
		t.Fatal(err)
	}
	m = query("host.home.test.", "tcp", wire.TypeA, true)
	if m.Answer[0].(*wire.A).A.String() != "192.0.2.2" {
		t.Fatal("old answer after mutation")
	}
	if err = local.Delete(ctx, z.ID, z.Revision); err != nil {
		t.Fatal(err)
	}
	m = query("host.home.test.", "udp", wire.TypeA, true)
	if m.Authoritative || m.Answer[0].(*wire.A).A.String() != "192.0.2.99" {
		t.Fatal("deleted zone still active")
	}
	m = query("outside.test.", "tcp", wire.TypeA, false)
	if m.Rcode != wire.RcodeRefused {
		t.Fatal("nonrecursive external request forwarded")
	}
}
