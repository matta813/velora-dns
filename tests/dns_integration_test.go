package tests

import (
	"context"
	"fmt"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	server "github.com/matta813/velora-dns/internal/dns"
	wire "github.com/miekg/dns"
)

func TestDNSUDPAndTCP(t *testing.T) {
	var calls atomic.Int32
	up, err := server.Start([]string{"127.0.0.1:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
		calls.Add(1)
		m := new(wire.Msg)
		m.SetReply(q)
		rr, e := wire.NewRR("example.test. 60 IN A 192.0.2.10")
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
	defer func() {
		if err := up.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	c := cache.New(10)
	h := &server.Handler{Context: context.Background(), Resolver: &server.Resolver{Cache: c, Forwarder: &server.Forwarder{Upstreams: up.Addresses(), Timeout: time.Second}}, Allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Slots: make(chan struct{}, 5)}
	s, err := server.Start([]string{"127.0.0.1:0"}, h)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	for _, network := range []string{"udp", "tcp"} {
		t.Run(network, func(t *testing.T) {
			q := new(wire.Msg)
			q.SetQuestion("example.test.", wire.TypeA)
			m, _, err := (&wire.Client{Net: network, Timeout: time.Second}).Exchange(q, s.Addresses()[0])
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Answer) != 1 || m.Answer[0].(*wire.A).A.String() != "192.0.2.10" || m.Id != q.Id {
				t.Fatalf("incorrect DNS response: %v", m)
			}
		})
	}
	if calls.Load() != 1 || c.Stats().Hits != 1 {
		t.Fatal("second transport did not reuse cache")
	}
}
func TestDNSClientDenied(t *testing.T) {
	h := &server.Handler{Context: context.Background(), Allowed: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}}
	s, err := server.Start([]string{"127.0.0.1:0"}, h)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Shutdown(context.Background()) }()
	q := new(wire.Msg)
	q.SetQuestion("example.test.", wire.TypeA)
	m, _, err := (&wire.Client{Timeout: time.Second}).Exchange(q, s.Addresses()[0])
	if err != nil || m.Rcode != wire.RcodeRefused {
		t.Fatalf("ACL not enforced: %v %v", m, err)
	}
}

type overloadCounter struct{ count atomic.Int32 }

func (o *overloadCounter) Overload(reason string) {
	if reason == "client_rate" {
		o.count.Add(1)
	}
}

func TestDNSRateLimitAppliesAcrossUDPAndTCP(t *testing.T) {
	var calls atomic.Int32
	up, err := server.Start([]string{"127.0.0.1:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
		calls.Add(1)
		m := new(wire.Msg)
		m.SetReply(q)
		m.Answer = []wire.RR{&wire.A{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 1}}}
		_ = w.WriteMsg(m)
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = up.Shutdown(context.Background()) }()
	overloads := &overloadCounter{}
	limiter := server.NewLimiter(1, 2, 100, 100, 10)
	h := &server.Handler{Context: context.Background(), Resolver: &server.Resolver{Cache: cache.New(0), Forwarder: &server.Forwarder{Upstreams: up.Addresses(), Timeout: time.Second}}, Allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Slots: make(chan struct{}, 5), Limiter: limiter, Overload: overloads}
	s, err := server.Start([]string{"127.0.0.1:0"}, h)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Shutdown(context.Background()) }()
	for i, network := range []string{"udp", "tcp", "udp"} {
		q := new(wire.Msg)
		q.SetQuestion(fmt.Sprintf("query-%d.test.", i), wire.TypeA)
		m, _, exchangeErr := (&wire.Client{Net: network, Timeout: time.Second}).Exchange(q, s.Addresses()[0])
		if exchangeErr != nil {
			t.Fatal(exchangeErr)
		}
		want := wire.RcodeSuccess
		if i == 2 {
			want = wire.RcodeRefused
		}
		if m.Rcode != want {
			t.Fatalf("query %d: got %s", i, wire.RcodeToString[m.Rcode])
		}
	}
	if calls.Load() != 2 || overloads.count.Load() != 1 {
		t.Fatalf("calls=%d overloads=%d", calls.Load(), overloads.count.Load())
	}
}

func TestForwardedRecordTypes(t *testing.T) {
	records := map[uint16]string{
		wire.TypeA: "192.0.2.20", wire.TypeAAAA: "2001:db8::20", wire.TypeCNAME: "target.example.test.",
		wire.TypeTXT: "\"test text\"", wire.TypeMX: "10 mail.example.test.", wire.TypeNS: "ns.example.test.", wire.TypePTR: "host.example.test.",
	}
	up, err := server.Start([]string{"127.0.0.1:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
		m := new(wire.Msg)
		m.SetReply(q)
		rr, e := wire.NewRR(q.Question[0].Name + " 60 IN " + wire.TypeToString[q.Question[0].Qtype] + " " + records[q.Question[0].Qtype])
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
	defer func() { _ = up.Shutdown(context.Background()) }()
	h := &server.Handler{Context: context.Background(), Resolver: &server.Resolver{Cache: cache.New(10), Forwarder: &server.Forwarder{Upstreams: up.Addresses(), Timeout: time.Second}}, Allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Slots: make(chan struct{}, 5)}
	s, err := server.Start([]string{"127.0.0.1:0"}, h)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Shutdown(context.Background()) }()
	for kind := range records {
		for _, network := range []string{"udp", "tcp"} {
			t.Run(wire.TypeToString[kind]+"/"+network, func(t *testing.T) {
				q := new(wire.Msg)
				q.SetQuestion("example.test.", kind)
				m, _, e := (&wire.Client{Net: network, Timeout: time.Second}).Exchange(q, s.Addresses()[0])
				if e != nil {
					t.Fatal(e)
				}
				if len(m.Answer) != 1 || m.Answer[0].Header().Rrtype != kind {
					t.Fatalf("incorrect type response: %v", m)
				}
			})
		}
	}
}

func TestSeparateIPv4AndIPv6Listeners(t *testing.T) {
	s, err := server.Start([]string{"127.0.0.1:0", "[::1]:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) { m := new(wire.Msg); m.SetReply(q); _ = w.WriteMsg(m) }))
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer func() { _ = s.Shutdown(context.Background()) }()
	for _, address := range s.Addresses() {
		q := new(wire.Msg)
		q.SetQuestion("test.", wire.TypeA)
		if _, _, err := (&wire.Client{Timeout: time.Second}).Exchange(q, address); err != nil {
			t.Fatal(err)
		}
	}
}
