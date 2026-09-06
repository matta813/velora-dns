package tests

import (
	"context"
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
