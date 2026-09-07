package dns

import (
	wire "github.com/miekg/dns"
	"net"
)

func (r *Resolver) blocked(q *wire.Msg) Result {
	m := new(wire.Msg)
	m.SetRcode(q, wire.RcodeNameError)
	m.RecursionAvailable = true
	if r.BlockMode == "ZERO" {
		m.Rcode = wire.RcodeSuccess
		header := wire.RR_Header{Name: q.Question[0].Name, Rrtype: q.Question[0].Qtype, Class: wire.ClassINET, Ttl: 0}
		switch header.Rrtype {
		case wire.TypeA:
			m.Answer = []wire.RR{&wire.A{Hdr: header, A: net.IPv4zero}}
		case wire.TypeAAAA:
			m.Answer = []wire.RR{&wire.AAAA{Hdr: header, AAAA: net.IPv6zero}}
		}
	}
	return Result{Message: m, Source: "blocked"}
}
func (r *Resolver) blockedAnswer(m *wire.Msg) bool {
	if r.Filter == nil {
		return false
	}
	for _, rr := range m.Answer {
		if r.Filter.Blocked(rr.Header().Name) {
			return true
		}
		if alias, ok := rr.(*wire.CNAME); ok && r.Filter.Blocked(alias.Target) {
			return true
		}
	}
	return false
}
