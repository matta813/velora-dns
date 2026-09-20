package dns

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/matta813/velora-dns/internal/querylog"
	wire "github.com/miekg/dns"
)

type QueryObserver interface {
	Query(kind, source string, rcode int, elapsed time.Duration)
}
type OverloadObserver interface{ Overload(reason string) }
type AuditLogger interface{ Record(querylog.Entry) }
type Handler struct {
	Context      context.Context
	Resolver     *Resolver
	Allowed      []netip.Prefix
	Slots        chan struct{}
	Observer     QueryObserver
	Audit        AuditLogger
	CookieSecret []byte
	RateLimit    *RateLimitState
	mu           sync.RWMutex
}

func (h *Handler) UpdateConfig(allowed []netip.Prefix, maxConcurrent int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Allowed = allowed
	h.Slots = make(chan struct{}, maxConcurrent)
}

func (h *Handler) ServeDNS(w wire.ResponseWriter, q *wire.Msg) {
	started := time.Now()
	source := "refused"
	upstream := ""
	clientIP := ""
	m := new(wire.Msg)
	m.SetReply(q)
	m.RecursionAvailable = true
	defer func() {
		if _, ok := w.RemoteAddr().(*net.UDPAddr); ok {
			size := uint16(512)
			if opt := q.IsEdns0(); opt != nil {
				size = opt.UDPSize()
				if size < 512 {
					size = 512
				}
				if size > 1232 {
					size = 1232
				}
			}
			m.Truncate(int(size))
		}
		_ = w.WriteMsg(m)
		if h.Observer != nil {
			kind := "other"
			if len(q.Question) == 1 {
				if name, ok := wire.TypeToString[q.Question[0].Qtype]; ok && supported(q.Question[0].Qtype) {
					kind = name
				}
			}
			h.Observer.Query(kind, source, m.Rcode, time.Since(started))
		}
		if h.Audit != nil && len(q.Question) == 1 {
			h.Audit.Record(querylog.Entry{OccurredAt: started, ClientIP: clientIP, Domain: q.Question[0].Name, Type: wire.TypeToString[q.Question[0].Qtype], Rcode: wire.RcodeToString[m.Rcode], Duration: time.Since(started), Source: source, Upstream: upstream, CacheHit: source == "cache"})
		}
	}()
	host, _, err := net.SplitHostPort(w.RemoteAddr().String())
	ip, e := netip.ParseAddr(host)
	clientIP = host
	allowed := false
	if err == nil && e == nil {
		h.mu.RLock()
		prefixes := h.Allowed
		h.mu.RUnlock()
		for _, prefix := range prefixes {
			if prefix.Contains(ip.Unmap()) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		m.Rcode = wire.RcodeRefused
		return
	}
	if h.RateLimit != nil && !h.RateLimit.Allow(ip.Unmap(), started) {
		m.Rcode = wire.RcodeServerFailure
		source = "rate_limit"
		h.overloaded("rate_limit")
		return
	}
	cookie, ednsRcode := clientEDNS(q, ip, h.CookieSecret)
	if ednsRcode != wire.RcodeSuccess {
		if q.IsEdns0() != nil {
			m.SetEdns0(1232, false)
		}
		m.SetRcode(q, ednsRcode)
		addResponseCookie(m, q, cookie)
		return
	}
	if q.Response || len(q.Question) != 1 || q.Opcode != wire.OpcodeQuery {
		m.Rcode = wire.RcodeFormatError
		return
	}
	question := q.Question[0]
	if !supported(question.Qtype) || question.Qclass != wire.ClassINET {
		m.Rcode = wire.RcodeRefused
		return
	}
	h.mu.RLock()
	slots := h.Slots
	h.mu.RUnlock()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	default:
		m.Rcode = wire.RcodeServerFailure
		source = "overload"
		h.overloaded("concurrency")
		return
	}
	ctx, cancel := context.WithTimeout(h.Context, 5*time.Second)
	defer cancel()
	result, err := h.Resolver.Resolve(ctx, q)
	source = result.Source
	upstream = result.Upstream
	if err != nil {
		m.Rcode = wire.RcodeServerFailure
		return
	}
	m = result.Message
	addResponseCookie(m, q, cookie)
}

func (h *Handler) overloaded(reason string) {
	if observer, ok := h.Observer.(OverloadObserver); ok {
		observer.Overload(reason)
	}
}
func supported(t uint16) bool {
	switch t {
	case wire.TypeA, wire.TypeAAAA, wire.TypeCNAME, wire.TypeTXT, wire.TypeMX, wire.TypeNS, wire.TypePTR, wire.TypeSOA:
		return true
	}
	return false
}
