package dns

import (
	"context"
	"net"
	"net/netip"
	"time"

	wire "github.com/miekg/dns"
)

type QueryObserver interface {
	Query(kind, source string, rcode int, elapsed time.Duration)
}
type Handler struct {
	Context  context.Context
	Resolver *Resolver
	Allowed  []netip.Prefix
	Slots    chan struct{}
	Observer QueryObserver
}

func (h *Handler) ServeDNS(w wire.ResponseWriter, q *wire.Msg) {
	started := time.Now()
	source := "refused"
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
	}()
	host, _, err := net.SplitHostPort(w.RemoteAddr().String())
	ip, e := netip.ParseAddr(host)
	allowed := false
	if err == nil && e == nil {
		for _, prefix := range h.Allowed {
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
	if q.Response || len(q.Question) != 1 || q.Opcode != wire.OpcodeQuery {
		m.Rcode = wire.RcodeFormatError
		return
	}
	question := q.Question[0]
	if !supported(question.Qtype) || question.Qclass != wire.ClassINET {
		m.Rcode = wire.RcodeRefused
		return
	}
	if opt := q.IsEdns0(); opt != nil && opt.Version() != 0 {
		m.SetEdns0(1232, false)
		m.Rcode = wire.RcodeBadVers
		return
	}
	select {
	case h.Slots <- struct{}{}:
		defer func() { <-h.Slots }()
	default:
		m.Rcode = wire.RcodeServerFailure
		source = "overload"
		return
	}
	ctx, cancel := context.WithTimeout(h.Context, 5*time.Second)
	defer cancel()
	result, err := h.Resolver.Resolve(ctx, q)
	source = result.Source
	if err != nil {
		m.Rcode = wire.RcodeServerFailure
		return
	}
	m = result.Message
}
func supported(t uint16) bool {
	switch t {
	case wire.TypeA, wire.TypeAAAA, wire.TypeCNAME, wire.TypeTXT, wire.TypeMX, wire.TypeNS, wire.TypePTR, wire.TypeSOA:
		return true
	}
	return false
}
