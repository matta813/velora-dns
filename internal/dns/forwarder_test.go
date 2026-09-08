package dns

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	wire "github.com/miekg/dns"
)

func upstream(t *testing.T, handler wire.HandlerFunc) *Server {
	t.Helper()
	s, err := Start([]string{"127.0.0.1:0"}, handler)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return s
}
func answer(w wire.ResponseWriter, q *wire.Msg) {
	m := new(wire.Msg)
	m.SetReply(q)
	m.Answer = []wire.RR{&wire.A{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 60}}}
	_ = w.WriteMsg(m)
}
func TestForwarderFailover(t *testing.T) {
	bad := upstream(t, func(w wire.ResponseWriter, q *wire.Msg) {
		m := new(wire.Msg)
		m.SetRcode(q, wire.RcodeServerFailure)
		_ = w.WriteMsg(m)
	})
	good := upstream(t, answer)
	f := Forwarder{Upstreams: []string{bad.Addresses()[0], good.Addresses()[0]}, Timeout: 100 * time.Millisecond}
	q := new(wire.Msg)
	q.SetQuestion("example.org.", wire.TypeA)
	m, used, err := f.Resolve(context.Background(), q)
	if err != nil || used != good.Addresses()[0] || m.Id != q.Id || len(m.Answer) != 1 {
		t.Fatalf("failover: %v %s %v", m, used, err)
	}
}

func TestForwarderRejectsMalformedCNAMEBeforeFailover(t *testing.T) {
	bad := upstream(t, func(w wire.ResponseWriter, q *wire.Msg) {
		m := new(wire.Msg)
		m.SetReply(q)
		m.Answer = []wire.RR{&wire.CNAME{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeCNAME, Class: wire.ClassINET, Ttl: 60}, Target: q.Question[0].Name}}
		_ = w.WriteMsg(m)
	})
	good := upstream(t, answer)
	q := new(wire.Msg)
	q.SetQuestion("alias.test.", wire.TypeA)
	m, used, err := (&Forwarder{Upstreams: []string{bad.Addresses()[0], good.Addresses()[0]}, Timeout: time.Second}).Resolve(context.Background(), q)
	if err != nil || used != good.Addresses()[0] || len(m.Answer) != 1 {
		t.Fatalf("malformed CNAME failover: %v %s %v", m, used, err)
	}
}
func TestRetryAndTCPFallback(t *testing.T) {
	var calls atomic.Int32
	s := upstream(t, func(w wire.ResponseWriter, q *wire.Msg) {
		n := calls.Add(1)
		m := new(wire.Msg)
		m.SetReply(q)
		switch n {
		case 1:
			m.Rcode = wire.RcodeServerFailure
		case 2:
			m.Truncated = true
		default:
			m.Answer = []wire.RR{&wire.TXT{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeTXT, Class: wire.ClassINET, Ttl: 10}, Txt: []string{"ok"}}}
		}
		_ = w.WriteMsg(m)
	})
	f := Forwarder{Upstreams: s.Addresses(), Timeout: time.Second, Retries: 1}
	q := new(wire.Msg)
	q.SetQuestion("test.", wire.TypeTXT)
	m, _, err := f.Resolve(context.Background(), q)
	if err != nil || len(m.Answer) != 1 || calls.Load() != 3 {
		t.Fatalf("retry/TCP fallback: %v %v", m, err)
	}
}
func TestForwarderCancellation(t *testing.T) {
	f := Forwarder{Upstreams: []string{"127.0.0.1:1"}, Timeout: time.Second, Retries: 3}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := new(wire.Msg)
	q.SetQuestion("test.", wire.TypeA)
	if _, _, err := f.Resolve(ctx, q); err == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestSupportedRecords(t *testing.T) {
	for _, kind := range []uint16{wire.TypeA, wire.TypeAAAA, wire.TypeCNAME, wire.TypeTXT, wire.TypeMX, wire.TypeNS, wire.TypePTR} {
		if !supported(kind) {
			t.Fatalf("type %d rejected", kind)
		}
	}
	if supported(wire.TypeANY) || supported(wire.TypeAXFR) {
		t.Fatal("unsafe type allowed")
	}
}

func TestQuestionMatching(t *testing.T) {
	q := new(wire.Msg)
	q.SetQuestion("Example.Test.", wire.TypeA)
	response := q.Copy()
	response.Question[0].Name = "example.test."
	if !sameQuestion(q, response) {
		t.Fatal("case-insensitive question should match")
	}
	response.Question[0].Name = "different.test."
	if sameQuestion(q, response) {
		t.Fatal("different name matched")
	}
	response = q.Copy()
	response.Question[0].Qtype = wire.TypeAAAA
	if sameQuestion(q, response) {
		t.Fatal("different type matched")
	}
	response = q.Copy()
	response.Question[0].Qclass = wire.ClassCHAOS
	if sameQuestion(q, response) {
		t.Fatal("different class matched")
	}
}

func TestLargeEDNSAnswerUsesTCP(t *testing.T) {
	var tcpCalls atomic.Int32
	s := upstream(t, func(w wire.ResponseWriter, q *wire.Msg) {
		m := new(wire.Msg)
		m.SetReply(q)
		m.Answer = []wire.RR{&wire.TXT{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeTXT, Class: wire.ClassINET, Ttl: 60}, Txt: []string{strings.Repeat("a", 250), strings.Repeat("b", 250), strings.Repeat("c", 250), strings.Repeat("d", 250), strings.Repeat("e", 250), strings.Repeat("f", 250)}}}
		if _, ok := w.RemoteAddr().(*net.UDPAddr); ok {
			m.Truncate(int(q.IsEdns0().UDPSize()))
		} else {
			tcpCalls.Add(1)
		}
		_ = w.WriteMsg(m)
	})
	q := new(wire.Msg)
	q.SetQuestion("large.test.", wire.TypeTXT)
	q.SetEdns0(4096, false)
	f := Forwarder{Upstreams: s.Addresses(), Timeout: time.Second}
	m, _, err := f.Resolve(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if m.Truncated || len(m.Answer) != 1 || tcpCalls.Load() != 1 {
		t.Fatalf("large answer failed TCP fallback: %v", m)
	}
	if q.IsEdns0().UDPSize() != 4096 {
		t.Fatal("forwarder mutated client message")
	}
}

func TestForwarderAppliesPrivacyPreservingEDNSPolicy(t *testing.T) {
	seen := make(chan *wire.Msg, 1)
	s := upstream(t, func(w wire.ResponseWriter, q *wire.Msg) {
		seen <- q.Copy()
		m := new(wire.Msg)
		m.SetReply(q)
		m.Answer = []wire.RR{&wire.A{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 60}}}
		m.SetEdns0(1232, true)
		m.IsEdns0().Option = append(m.IsEdns0().Option, &wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: "0102030405060708"})
		_ = w.WriteMsg(m)
	})
	q := new(wire.Msg)
	q.SetQuestion("privacy.test.", wire.TypeA)
	q.SetEdns0(4096, true)
	q.IsEdns0().Option = append(q.IsEdns0().Option,
		&wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: "0102030405060708"},
		&wire.EDNS0_SUBNET{Code: wire.EDNS0SUBNET, Family: 1, SourceNetmask: 24, Address: net.ParseIP("192.0.2.1")},
	)
	m, _, err := (&Forwarder{Upstreams: s.Addresses(), Timeout: time.Second}).Resolve(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	forwarded := <-seen
	if opt := forwarded.IsEdns0(); opt == nil || opt.UDPSize() != 1232 || !opt.Do() || len(opt.Option) != 0 {
		t.Fatalf("forwarded EDNS policy: %v", opt)
	}
	if len(m.IsEdns0().Option) != 0 {
		t.Fatal("upstream cookie leaked to client")
	}
	if len(q.IsEdns0().Option) != 2 || q.IsEdns0().UDPSize() != 4096 {
		t.Fatal("client query was mutated")
	}
}
