package tests

import (
	"context"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/cache"
	server "github.com/matta813/velora-dns/internal/dns"
	wire "github.com/miekg/dns"
)

func TestDNSCookieChallengeAndReplayProtection(t *testing.T) {
	var calls atomic.Int32
	upstream, err := server.Start([]string{"127.0.0.1:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
		calls.Add(1)
		m := new(wire.Msg)
		m.SetReply(q)
		m.Answer = []wire.RR{&wire.A{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 60}}}
		_ = w.WriteMsg(m)
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = upstream.Shutdown(context.Background()) }()
	handler := &server.Handler{
		Context: context.Background(), Resolver: &server.Resolver{Cache: cache.New(10), Forwarder: &server.Forwarder{Upstreams: upstream.Addresses(), Timeout: time.Second}},
		Allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Slots: make(chan struct{}, 2), CookieSecret: []byte("integration cookie secret"),
	}
	dnsServer, err := server.Start([]string{"127.0.0.1:0"}, handler)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dnsServer.Shutdown(context.Background()) }()

	query := new(wire.Msg)
	query.SetQuestion("cookie.test.", wire.TypeA)
	query.SetEdns0(1232, false)
	query.IsEdns0().Option = append(query.IsEdns0().Option, &wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: "0102030405060708"})
	client := &wire.Client{Net: "udp", Timeout: time.Second}
	response, _, err := client.Exchange(query, dnsServer.Addresses()[0])
	if err != nil || response.Rcode != wire.RcodeSuccess || len(response.Answer) != 1 {
		t.Fatalf("initial cookie query: %v %v", response, err)
	}
	cookie := response.IsEdns0().Option[0].(*wire.EDNS0_COOKIE).Cookie
	query.IsEdns0().Option[0] = &wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: cookie}
	response, _, err = client.Exchange(query, dnsServer.Addresses()[0])
	if err != nil || response.Rcode != wire.RcodeSuccess {
		t.Fatalf("valid cookie query: %v %v", response, err)
	}
	query.IsEdns0().Option[0] = &wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: cookie[:len(cookie)-2] + "00"}
	response, _, err = client.Exchange(query, dnsServer.Addresses()[0])
	if err != nil || response.Rcode != wire.RcodeBadCookie {
		t.Fatalf("bad cookie response: %v %v", response, err)
	}
	if response.IsEdns0() == nil || !strings.HasPrefix(response.IsEdns0().Option[0].(*wire.EDNS0_COOKIE).Cookie, "0102030405060708") {
		t.Fatal("BADCOOKIE response omitted replacement cookie")
	}
	if calls.Load() != 2 {
		t.Fatalf("invalid cookie reached resolver: %d calls", calls.Load())
	}
}

func TestSOABackedNegativeResponsesAreCached(t *testing.T) {
	for _, rcode := range []int{wire.RcodeNameError, wire.RcodeSuccess} {
		t.Run(wire.RcodeToString[rcode], func(t *testing.T) {
			var calls atomic.Int32
			upstream, err := server.Start([]string{"127.0.0.1:0"}, wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
				calls.Add(1)
				m := new(wire.Msg)
				m.SetRcode(q, rcode)
				m.Ns = []wire.RR{&wire.SOA{Hdr: wire.RR_Header{Name: "test.", Rrtype: wire.TypeSOA, Class: wire.ClassINET, Ttl: 300}, Ns: "ns.test.", Mbox: "hostmaster.test.", Serial: 1, Refresh: 3600, Retry: 600, Expire: 86400, Minttl: 60}}
				_ = w.WriteMsg(m)
			}))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = upstream.Shutdown(context.Background()) }()
			handler := &server.Handler{Context: context.Background(), Resolver: &server.Resolver{Cache: cache.New(10), Forwarder: &server.Forwarder{Upstreams: upstream.Addresses(), Timeout: time.Second}}, Allowed: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Slots: make(chan struct{}, 2)}
			dnsServer, err := server.Start([]string{"127.0.0.1:0"}, handler)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = dnsServer.Shutdown(context.Background()) }()
			q := new(wire.Msg)
			q.SetQuestion("missing.test.", wire.TypeA)
			client := &wire.Client{Net: "udp", Timeout: time.Second}
			for range 2 {
				m, _, exchangeErr := client.Exchange(q, dnsServer.Addresses()[0])
				if exchangeErr != nil || m.Rcode != rcode || len(m.Ns) != 1 || m.Ns[0].Header().Ttl > 60 {
					t.Fatalf("negative response: %v %v", m, exchangeErr)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("negative response was not cached: %d upstream calls", calls.Load())
			}
		})
	}
}
