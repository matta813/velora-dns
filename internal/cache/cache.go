// Package cache provides bounded positive and RFC 2308 negative DNS caching.
package cache

import (
	"container/list"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type entry struct {
	key             string
	message         *dns.Msg
	stored, expires time.Time
}
type Stats struct {
	Entries  int    `json:"entries"`
	Capacity int    `json:"capacity"`
	Hits     uint64 `json:"hits"`
	Misses   uint64 `json:"misses"`
}
type Cache struct {
	mu           sync.Mutex
	items        map[string]*list.Element
	lru          *list.List
	max          int
	now          func() time.Time
	hits, misses uint64
}

func New(max int) *Cache {
	if max < 0 {
		max = 0
	}
	return &Cache{items: make(map[string]*list.Element), lru: list.New(), max: max, now: time.Now}
}
func Key(q *dns.Msg) (string, bool) {
	if len(q.Question) != 1 || q.Opcode != dns.OpcodeQuery || len(q.Answer) > 0 || len(q.Ns) > 0 {
		return "", false
	}
	do := false
	for _, rr := range q.Extra {
		opt, ok := rr.(*dns.OPT)
		if !ok || len(opt.Option) > 0 || opt.Version() != 0 {
			return "", false
		}
		do = opt.Do()
	}
	question := q.Question[0]
	return fmt.Sprintf("%s/%d/%d/%t/%t/%t", strings.ToLower(question.Name), question.Qtype, question.Qclass, q.RecursionDesired, q.CheckingDisabled, do), true
}
func (c *Cache) Get(q *dns.Msg) (*dns.Msg, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key, ok := Key(q)
	el := c.items[key]
	if !ok || el == nil {
		c.misses++
		return nil, false
	}
	e := el.Value.(*entry)
	now := c.now()
	if !now.Before(e.expires) {
		c.remove(el)
		c.misses++
		return nil, false
	}
	c.hits++
	c.lru.MoveToFront(el)
	m := e.message.Copy()
	m.Id = q.Id
	m.Question = append([]dns.Question(nil), q.Question...)
	age := uint32(now.Sub(e.stored) / time.Second)
	for _, section := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
		for _, rr := range section {
			if rr.Header().Rrtype != dns.TypeOPT {
				rr.Header().Ttl -= age
			}
		}
	}
	return m, true
}
func (c *Cache) Put(q, m *dns.Msg) {
	key, ok := Key(q)
	if !ok || c.max == 0 || m.Truncated || m.Len() > 16384 {
		return
	}
	stored := m.Copy()
	ttl, negative := cacheTTL(stored)
	if ttl == 0 {
		return
	}
	if negative {
		clampNegativeSOA(stored, ttl)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old := c.items[key]; old != nil {
		c.remove(old)
	}
	if len(c.items) >= c.max {
		c.remove(c.lru.Back())
	}
	now := c.now()
	e := &entry{key: key, message: stored, stored: now, expires: now.Add(time.Duration(ttl) * time.Second)}
	c.items[key] = c.lru.PushFront(e)
}

// NormalizeNegativeTTL advertises the actual RFC 2308 cache lifetime on the wire.
func NormalizeNegativeTTL(m *dns.Msg) {
	if ttl, negative := cacheTTL(m); negative {
		clampNegativeSOA(m, ttl)
	}
}

func clampNegativeSOA(m *dns.Msg, ttl uint32) {
	for _, rr := range m.Ns {
		if soa, yes := rr.(*dns.SOA); yes {
			soa.Hdr.Ttl = ttl
		}
	}
}

func cacheTTL(m *dns.Msg) (uint32, bool) {
	const maxTTL = uint32(86400)
	var soa *dns.SOA
	for _, rr := range m.Ns {
		if candidate, yes := rr.(*dns.SOA); yes {
			soa = candidate
			break
		}
	}
	aliasOnly := true
	for _, rr := range m.Answer {
		if rr.Header().Rrtype != dns.TypeCNAME && rr.Header().Rrtype != dns.TypeRRSIG {
			aliasOnly = false
			break
		}
	}
	negative := soa != nil && (m.Rcode == dns.RcodeNameError || (m.Rcode == dns.RcodeSuccess && (len(m.Answer) == 0 || aliasOnly)))
	if negative {
		ttl := min(soa.Hdr.Ttl, soa.Minttl, maxTTL)
		for _, rr := range m.Answer {
			if rr.Header().Ttl < ttl {
				ttl = rr.Header().Ttl
			}
		}
		return ttl, true
	}
	if m.Rcode == dns.RcodeSuccess && len(m.Answer) > 0 {
		ttl := maxTTL
		for _, section := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
			for _, rr := range section {
				if rr.Header().Rrtype != dns.TypeOPT && rr.Header().Ttl < ttl {
					ttl = rr.Header().Ttl
				}
			}
		}
		return ttl, false
	}
	return 0, false
}
func (c *Cache) remove(el *list.Element) { delete(c.items, el.Value.(*entry).key); c.lru.Remove(el) }
func (c *Cache) expire() {
	now := c.now()
	for _, el := range c.items {
		if !now.Before(el.Value.(*entry).expires) {
			c.remove(el)
		}
	}
}
func (c *Cache) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			c.expire()
			c.mu.Unlock()
		}
	}
}
func (c *Cache) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expire()
	return Stats{Entries: len(c.items), Capacity: c.max, Hits: c.hits, Misses: c.misses}
}
func (c *Cache) Flush() { c.mu.Lock(); defer c.mu.Unlock(); clear(c.items); c.lru.Init() }
