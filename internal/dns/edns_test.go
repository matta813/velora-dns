package dns

import (
	"encoding/hex"
	"net/netip"
	"testing"

	wire "github.com/miekg/dns"
)

func TestClientEDNSCookieLifecycle(t *testing.T) {
	q := new(wire.Msg)
	q.SetQuestion("example.test.", wire.TypeA)
	q.SetEdns0(4096, false)
	q.IsEdns0().Option = append(q.IsEdns0().Option, &wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: "0102030405060708"})
	client := netip.MustParseAddr("192.0.2.1")
	response, rcode := clientEDNS(q, client, []byte("test secret"))
	if rcode != wire.RcodeSuccess {
		t.Fatalf("client cookie rejected: %d", rcode)
	}
	decoded, err := hex.DecodeString(response.Cookie)
	if err != nil || len(decoded) != 8+serverCookieBytes {
		t.Fatalf("response cookie: %q %v", response.Cookie, err)
	}
	q.IsEdns0().Option[0] = response
	if _, rcode = clientEDNS(q, client, []byte("test secret")); rcode != wire.RcodeSuccess {
		t.Fatalf("valid server cookie rejected: %d", rcode)
	}
	if _, rcode = clientEDNS(q, netip.MustParseAddr("192.0.2.2"), []byte("test secret")); rcode != wire.RcodeBadCookie {
		t.Fatalf("cookie replay accepted: %d", rcode)
	}
}

func TestClientEDNSRejectsMalformedStructure(t *testing.T) {
	base := new(wire.Msg)
	base.SetQuestion("example.test.", wire.TypeA)
	for _, mutate := range []func(*wire.Msg){
		func(q *wire.Msg) { q.SetEdns0(1232, false); q.IsEdns0().Hdr.Name = "bad.test." },
		func(q *wire.Msg) {
			q.SetEdns0(1232, false)
			q.Extra = append(q.Extra, &wire.OPT{Hdr: wire.RR_Header{Name: ".", Rrtype: wire.TypeOPT}})
		},
		func(q *wire.Msg) {
			q.SetEdns0(1232, false)
			q.IsEdns0().Option = append(q.IsEdns0().Option, &wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: "00"})
		},
		func(q *wire.Msg) {
			q.SetEdns0(1232, false)
			q.IsEdns0().Option = append(q.IsEdns0().Option, &wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: "0102030405060708"}, &wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: "0102030405060708"})
		},
	} {
		q := base.Copy()
		mutate(q)
		if _, rcode := clientEDNS(q, netip.MustParseAddr("192.0.2.1"), []byte("secret")); rcode != wire.RcodeFormatError {
			t.Fatalf("malformed EDNS accepted: %v", q)
		}
	}
}

func TestValidateAnswerChain(t *testing.T) {
	q := new(wire.Msg)
	q.SetQuestion("alias.test.", wire.TypeA)
	valid := new(wire.Msg)
	valid.SetReply(q)
	valid.Answer = []wire.RR{
		&wire.CNAME{Hdr: wire.RR_Header{Name: "alias.test.", Rrtype: wire.TypeCNAME}, Target: "target.test."},
		&wire.A{Hdr: wire.RR_Header{Name: "target.test.", Rrtype: wire.TypeA}},
	}
	if err := validateAnswerChain(q, valid); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []*wire.Msg{
		{Answer: []wire.RR{&wire.A{Hdr: wire.RR_Header{Name: "unrelated.test.", Rrtype: wire.TypeA}}}},
		{Answer: []wire.RR{&wire.CNAME{Hdr: wire.RR_Header{Name: "alias.test.", Rrtype: wire.TypeCNAME}, Target: "alias.test."}}},
	} {
		if err := validateAnswerChain(q, bad); err == nil {
			t.Fatal("invalid answer chain accepted")
		}
	}
}
