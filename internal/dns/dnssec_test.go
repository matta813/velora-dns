package dns

import (
	"context"
	"crypto"
	"fmt"
	"strings"
	"testing"
	"time"

	wire "github.com/miekg/dns"
)

func TestDNSSECValidationAndCDSemantics(t *testing.T) {
	key, signer := testDNSSECKey(t, "example.")
	anchor := key.ToDS(wire.SHA256).String()
	retiring := *key.ToDS(wire.SHA256)
	retiring.Digest = strings.Repeat("0", len(retiring.Digest))
	validator, err := NewDNSSECValidator([]string{retiring.String(), anchor})
	if err != nil {
		t.Fatal(err)
	}
	server := upstream(t, func(w wire.ResponseWriter, q *wire.Msg) {
		m := new(wire.Msg)
		m.SetReply(q)
		switch q.Question[0].Qtype {
		case wire.TypeDNSKEY:
			m.Answer = signedSet(t, []wire.RR{key}, key, signer)
		default:
			answer := &wire.A{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 300}}
			answer.A = []byte{192, 0, 2, 1}
			m.Answer = signedSet(t, []wire.RR{answer}, key, signer)
		}
		_ = w.WriteMsg(m)
	})
	forwarder := &Forwarder{Upstreams: server.Addresses(), Timeout: time.Second, Validator: validator}

	query := question("www.example.", wire.TypeA)
	query.CheckingDisabled = false
	response, _, err := forwarder.Resolve(context.Background(), query)
	if err != nil || !response.AuthenticatedData {
		t.Fatalf("secure response: AD=%t err=%v", response.AuthenticatedData, err)
	}
	query.CheckingDisabled = true
	response, _, err = forwarder.Resolve(context.Background(), query)
	if err != nil || response.AuthenticatedData {
		t.Fatalf("CD response: AD=%t err=%v", response.AuthenticatedData, err)
	}
}

func TestDNSSECBogusResponseFails(t *testing.T) {
	key, signer := testDNSSECKey(t, "example.")
	validator, err := NewDNSSECValidator([]string{key.ToDS(wire.SHA256).String()})
	if err != nil {
		t.Fatal(err)
	}
	server := upstream(t, func(w wire.ResponseWriter, q *wire.Msg) {
		m := new(wire.Msg)
		m.SetReply(q)
		if q.Question[0].Qtype == wire.TypeDNSKEY {
			m.Answer = signedSet(t, []wire.RR{key}, key, signer)
		} else {
			answer := &wire.A{Hdr: wire.RR_Header{Name: q.Question[0].Name, Rrtype: wire.TypeA, Class: wire.ClassINET, Ttl: 300}, A: []byte{192, 0, 2, 1}}
			m.Answer = signedSet(t, []wire.RR{answer}, key, signer)
			answer.A = []byte{192, 0, 2, 99}
		}
		_ = w.WriteMsg(m)
	})
	query := question("www.example.", wire.TypeA)
	query.CheckingDisabled = false
	if _, _, err = (&Forwarder{Upstreams: server.Addresses(), Timeout: time.Second, Validator: validator}).Resolve(context.Background(), query); err == nil {
		t.Fatal("bogus signature was accepted")
	}
}

func TestAuthenticatedNSECDenial(t *testing.T) {
	key, signer := testDNSSECKey(t, "example.")
	validator, err := NewDNSSECValidator([]string{key.ToDS(wire.SHA256).String()})
	if err != nil {
		t.Fatal(err)
	}
	query := question("missing.example.", wire.TypeA)
	response := new(wire.Msg)
	response.SetRcode(query, wire.RcodeNameError)
	soa, _ := wire.NewRR("example. 300 IN SOA ns.example. hostmaster.example. 1 3600 600 86400 300")
	nsec, _ := wire.NewRR("a.example. 300 IN NSEC z.example. A RRSIG NSEC")
	closest, _ := wire.NewRR("example. 300 IN NSEC a.example. SOA RRSIG NSEC")
	response.Ns = append(signedSet(t, []wire.RR{soa}, key, signer), signedSet(t, []wire.RR{nsec}, key, signer)...)
	response.Ns = append(response.Ns, signedSet(t, []wire.RR{closest}, key, signer)...)
	status, err := validator.Validate(context.Background(), query, response, func(_ context.Context, q *wire.Msg) (*wire.Msg, error) {
		if q.Question[0].Qtype != wire.TypeDNSKEY {
			return nil, fmt.Errorf("unexpected query")
		}
		m := new(wire.Msg)
		m.SetReply(q)
		m.Answer = signedSet(t, []wire.RR{key}, key, signer)
		return m, nil
	})
	if err != nil || status != DNSSECSecure {
		t.Fatalf("authenticated denial: status=%d err=%v", status, err)
	}
}

func testDNSSECKey(t *testing.T, zone string) (*wire.DNSKEY, crypto.Signer) {
	t.Helper()
	key := &wire.DNSKEY{Hdr: wire.RR_Header{Name: zone, Rrtype: wire.TypeDNSKEY, Class: wire.ClassINET, Ttl: 300}, Flags: wire.ZONE | wire.SEP, Protocol: 3, Algorithm: wire.ECDSAP256SHA256}
	privateKey, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	signer, ok := privateKey.(crypto.Signer)
	if !ok {
		t.Fatal("generated key is not a signer")
	}
	return key, signer
}

func signedSet(t *testing.T, set []wire.RR, key *wire.DNSKEY, signer crypto.Signer) []wire.RR {
	t.Helper()
	now := time.Now()
	sig := &wire.RRSIG{Hdr: wire.RR_Header{Name: set[0].Header().Name, Rrtype: wire.TypeRRSIG, Class: wire.ClassINET, Ttl: set[0].Header().Ttl}, TypeCovered: set[0].Header().Rrtype, Algorithm: key.Algorithm, Labels: uint8(wire.CountLabel(set[0].Header().Name)), OrigTtl: set[0].Header().Ttl, Expiration: uint32(now.Add(time.Hour).Unix()), Inception: uint32(now.Add(-time.Hour).Unix()), KeyTag: key.KeyTag(), SignerName: key.Hdr.Name}
	if err := sig.Sign(signer, set); err != nil {
		t.Fatal(err)
	}
	return append(set, sig)
}
