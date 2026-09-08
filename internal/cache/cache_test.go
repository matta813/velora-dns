package cache

import (
	"context"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func pair(name string, ttl uint32) (*dns.Msg, *dns.Msg) {
	q := new(dns.Msg)
	q.SetQuestion(name, dns.TypeA)
	m := new(dns.Msg)
	m.SetReply(q)
	m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}}}
	return q, m
}
func TestTTLExpiryAndIsolation(t *testing.T) {
	c := New(2)
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	q, m := pair("Example.org.", 10)
	c.Put(q, m)
	m.Answer[0].Header().Ttl = 1
	now = now.Add(3 * time.Second)
	q.Id = 123
	q.Question[0].Name = "example.org."
	got, ok := c.Get(q)
	if !ok || got.Id != 123 || got.Answer[0].Header().Ttl != 7 {
		t.Fatalf("bad cached response: %v", got)
	}
	got.Answer[0].Header().Ttl = 0
	again, _ := c.Get(q)
	if again.Answer[0].Header().Ttl != 7 {
		t.Fatal("shared cached message")
	}
	now = now.Add(7 * time.Second)
	if _, ok = c.Get(q); ok || c.Stats().Entries != 0 {
		t.Fatal("expired entry survived")
	}
}
func TestLRUAndFlush(t *testing.T) {
	c := New(2)
	a, ar := pair("a.test.", 10)
	b, br := pair("b.test.", 10)
	d, dr := pair("d.test.", 10)
	c.Put(a, ar)
	c.Put(b, br)
	c.Get(a)
	c.Put(d, dr)
	if _, ok := c.Get(b); ok {
		t.Fatal("LRU entry retained")
	}
	if c.Stats().Entries != 2 {
		t.Fatal("capacity exceeded")
	}
	c.Flush()
	if c.Stats().Entries != 0 {
		t.Fatal("flush failed")
	}
}
func TestUnsafeResponsesNotCached(t *testing.T) {
	for _, kind := range []string{"zero", "truncated", "negative-without-soa", "options", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			c := New(1)
			q, m := pair("test.", 1)
			switch kind {
			case "zero":
				m.Answer[0].Header().Ttl = 0
			case "truncated":
				m.Truncated = true
			case "negative-without-soa":
				m.Rcode = dns.RcodeNameError
			case "options":
				q.SetEdns0(1232, false)
				q.IsEdns0().Option = append(q.IsEdns0().Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID})
			case "disabled":
				c = New(0)
			}
			c.Put(q, m)
			if _, ok := c.Get(q); ok {
				t.Fatal("unsafe entry cached")
			}
		})
	}
}

func TestRFC2308NegativeCaching(t *testing.T) {
	for _, rcode := range []int{dns.RcodeNameError, dns.RcodeSuccess} {
		t.Run(dns.RcodeToString[rcode], func(t *testing.T) {
			c := New(2)
			now := time.Unix(1000, 0)
			c.now = func() time.Time { return now }
			q, m := pair("missing.test.", 60)
			m.Answer = nil
			m.Rcode = rcode
			m.Ns = []dns.RR{&dns.SOA{Hdr: dns.RR_Header{Name: "test.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 600}, Minttl: 30}}
			c.Put(q, m)
			now = now.Add(10 * time.Second)
			got, ok := c.Get(q)
			if !ok || got.Rcode != rcode || got.Ns[0].Header().Ttl != 20 {
				t.Fatalf("negative cache: %v", got)
			}
			now = now.Add(20 * time.Second)
			if _, ok = c.Get(q); ok {
				t.Fatal("negative entry survived minimum SOA TTL")
			}
		})
	}
}

func TestNegativeTTLZeroAndCap(t *testing.T) {
	q, m := pair("missing.test.", 1)
	m.Answer = nil
	m.Rcode = dns.RcodeNameError
	m.Ns = []dns.RR{&dns.SOA{Hdr: dns.RR_Header{Rrtype: dns.TypeSOA, Ttl: 200000}, Minttl: 200000}}
	if ttl, negative := cacheTTL(m); ttl != 86400 || !negative {
		t.Fatalf("cap: %d %t", ttl, negative)
	}
	m.Ns[0].(*dns.SOA).Minttl = 0
	c := New(1)
	c.Put(q, m)
	if _, ok := c.Get(q); ok {
		t.Fatal("zero negative TTL cached")
	}
}
func TestKeySeparatesDNSSECFlags(t *testing.T) {
	q, _ := pair("test.", 5)
	a, _ := Key(q)
	q.CheckingDisabled = true
	b, _ := Key(q)
	q.SetEdns0(1232, true)
	c, _ := Key(q)
	if a == b || b == c {
		t.Fatal("key collision")
	}
}
func TestAutomaticExpiry(t *testing.T) {
	c := New(1)
	q, m := pair("test.", 1)
	c.Put(q, m)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.items)
		c.mu.Unlock()
		if n == 0 {
			cancel()
			<-done
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("sweeper did not expire entry")
}
