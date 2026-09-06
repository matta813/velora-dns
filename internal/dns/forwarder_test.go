package dns

import (
	"context"
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
