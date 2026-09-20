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
func TestConfiguredUpstreamTTL(t *testing.T) {
	c := New(2)
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	c.SetUpstreamTTL(86400)
	q, answer := pair("example.test.", 60)
	c.NormalizeUpstreamTTL(answer)
	if answer.Answer[0].Header().Ttl != 86400 {
		t.Fatalf("response TTL = %d", answer.Answer[0].Header().Ttl)
	}
	c.Put(q, answer)
	now = now.Add(time.Hour)
	got, ok := c.Get(q)
	if !ok || got.Answer[0].Header().Ttl != 82800 {
		t.Fatalf("cached TTL after one hour: %v, %t", got, ok)
	}
	c.SetUpstreamTTL(3600)
	if c.Stats().Entries != 0 {
		t.Fatal("old TTL policy entries survived configuration change")
	}
}

func TestUpstreamTTLLeavesZeroAndSignedAnswersAlone(t *testing.T) {
	c := New(2)
	c.SetUpstreamTTL(86400)
	_, zero := pair("zero.test.", 0)
	c.NormalizeUpstreamTTL(zero)
	if zero.Answer[0].Header().Ttl != 0 {
		t.Fatal("zero TTL was extended")
	}
	_, signed := pair("signed.test.", 60)
	signed.Answer = append(signed.Answer, &dns.RRSIG{Hdr: dns.RR_Header{Name: "signed.test.", Rrtype: dns.TypeRRSIG, Class: dns.ClassINET, Ttl: 60}})
	c.NormalizeUpstreamTTL(signed)
	if signed.Answer[0].Header().Ttl != 60 {
		t.Fatal("signed response TTL was extended")
	}
	_, negative := pair("missing.test.", 60)
	negative.Answer = nil
	negative.Rcode = dns.RcodeNameError
	negative.Ns = []dns.RR{&dns.SOA{Hdr: dns.RR_Header{Name: "test.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 60}, Minttl: 30}}
	c.NormalizeUpstreamTTL(negative)
	if negative.Ns[0].Header().Ttl != 60 {
		t.Fatal("negative response TTL was extended")
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
func TestListEntriesPaginatesAndExpires(t *testing.T) {
	c := New(3)
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	for _, name := range []string{"a.test.", "b.test.", "c.test."} {
		q, answer := pair(name, 10)
		c.Put(q, answer)
	}
	page := c.ListEntries(1, 1)
	if page.Total != 3 || len(page.Entries) != 1 || page.Entries[0].Name != "b.test." || page.Entries[0].Type != "A" || page.Entries[0].RemainingTTL != 10 {
		t.Fatalf("unexpected page: %+v", page)
	}
	now = now.Add(10 * time.Second)
	page = c.ListEntries(10, 0)
	if page.Total != 0 || len(page.Entries) != 0 {
		t.Fatalf("expired entries listed: %+v", page)
	}
}
func TestUnsafeResponsesNotCached(t *testing.T) {
	for _, kind := range []string{"zero", "truncated", "negative-without-soa", "disabled"} {
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
