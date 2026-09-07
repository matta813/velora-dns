package zones

import (
	"errors"
	"strings"
	"testing"

	wire "github.com/miekg/dns"
)

func TestRecordValidation(t *testing.T) {
	for _, tc := range []struct{ kind, value string }{{"A", "192.0.2.1"}, {"AAAA", "2001:db8::1"}, {"CNAME", "other.test"}, {"MX", "mail.test"}, {"NS", "ns.test"}, {"PTR", "host.test"}, {"TXT", strings.Repeat("ü", 300)}} {
		t.Run(tc.kind, func(t *testing.T) {
			name := "host"
			if tc.kind == "NS" {
				name = "@"
			}
			z, err := normalize(Zone{Name: "HOME.TEST", Records: []Record{{Name: name, Type: tc.kind, TTL: 60, Value: tc.value}}})
			if err != nil {
				t.Fatal(err)
			}
			rr, err := recordRR(z.Records[0])
			if err != nil {
				t.Fatal(err)
			}
			if rr.Header().Name != strings.ToLower(rr.Header().Name) {
				t.Fatal("owner not normalized")
			}
			if txt, ok := rr.(*wire.TXT); ok {
				if strings.Join(txt.Txt, "") != tc.value {
					t.Fatal("TXT changed")
				}
				for _, part := range txt.Txt {
					if len(part) > 255 {
						t.Fatal("oversized TXT chunk")
					}
				}
			}
		})
	}
	for _, record := range []Record{{Name: "outside.test.", Type: "A", Value: "192.0.2.1"}, {Name: "*", Type: "A", Value: "192.0.2.1"}, {Name: "a", Type: "A", Value: "::1"}, {Name: "a", Type: "AAAA", Value: "127.0.0.1"}, {Name: "@", Type: "CNAME", Value: "target.test"}, {Name: "child", Type: "NS", Value: "ns.test"}, {Name: "a", Type: "TXT", Value: strings.Repeat("x", 2049)}, {Name: "a", Type: "A", TTL: 86401, Value: "192.0.2.1"}, {Name: "a", Type: "TXT", Priority: 1}, {Name: "a", Type: "SRV"}} {
		if _, err := normalize(Zone{Name: "home.test", Records: []Record{record}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %+v: %v", record, err)
		}
	}
}
func TestRecordConflicts(t *testing.T) {
	cases := [][]Record{
		{{Name: "a", Type: "CNAME", Value: "b.home.test"}, {Name: "a", Type: "A", Value: "192.0.2.1"}},
		{{Name: "a", Type: "CNAME", Value: "b.home.test"}, {Name: "b", Type: "CNAME", Value: "a.home.test"}},
		{{Name: "a", Type: "A", Value: "192.0.2.1", TTL: 1}, {Name: "a", Type: "A", Value: "192.0.2.2", TTL: 2}},
		{{Name: "a", Type: "A", Value: "192.0.2.1"}, {Name: "a", Type: "A", Value: "192.0.2.1"}},
	}
	for _, records := range cases {
		if _, err := compile([]Zone{{Name: "home.test", Records: records}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected conflict: %v", err)
		}
	}
	_, err := compile([]Zone{{ID: 1, Name: "one.test", Records: []Record{{Name: "a", Type: "CNAME", Value: "b.two.test"}}}, {ID: 2, Name: "two.test", Records: []Record{{Name: "b", Type: "CNAME", Value: "a.one.test"}}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("cross-zone CNAME loop accepted")
	}
}
func TestAuthoritativeLookup(t *testing.T) {
	compiled, err := compile([]Zone{{ID: 1, Name: "home.test", Revision: 7, Records: []Record{
		{Name: "host", Type: "A", Value: "192.0.2.1", TTL: 60}, {Name: "deep.branch", Type: "TXT", Value: "text", TTL: 60},
		{Name: "alias", Type: "CNAME", Value: "host.home.test", TTL: 60}, {Name: "child", Type: "A", Value: "192.0.2.9", TTL: 60},
	}}, {ID: 2, Name: "child.home.test", Revision: 1, Records: []Record{}}})
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	s.state.Store(compiled)
	for _, tc := range []struct {
		name                     string
		kind                     uint16
		code, answers, authority int
		found                    bool
	}{
		{"HOST.HOME.TEST.", wire.TypeA, 0, 1, 0, true}, {"host.home.test.", wire.TypeAAAA, 0, 0, 1, true},
		{"missing.home.test.", wire.TypeA, wire.RcodeNameError, 0, 1, true}, {"branch.home.test.", wire.TypeA, 0, 0, 1, true},
		{"alias.home.test.", wire.TypeA, 0, 2, 0, true}, {"alias.home.test.", wire.TypeCNAME, 0, 1, 0, true},
		{"home.test.", wire.TypeSOA, 0, 1, 0, true}, {"home.test.", wire.TypeNS, 0, 1, 0, true},
		{"child.home.test.", wire.TypeA, 0, 0, 1, true}, {"nothome.test.", wire.TypeA, 0, 0, 0, false},
	} {
		t.Run(tc.name+wire.TypeToString[tc.kind], func(t *testing.T) {
			q := new(wire.Msg)
			q.SetQuestion(tc.name, tc.kind)
			m, found := s.Lookup(q)
			if found != tc.found {
				t.Fatalf("found=%v", found)
			}
			if !found {
				return
			}
			if !m.Authoritative || m.Id != q.Id || m.Rcode != tc.code || len(m.Answer) != tc.answers || len(m.Ns) != tc.authority {
				t.Fatalf("wrong response: %v", m)
			}
			if len(m.Ns) > 0 {
				soa := m.Ns[0].(*wire.SOA)
				if soa.Hdr.Ttl != 60 || soa.Minttl != 60 {
					t.Fatal("invalid negative TTL")
				}
			}
		})
	}
	q := new(wire.Msg)
	q.SetQuestion("host.home.test.", wire.TypeA)
	m, _ := s.Lookup(q)
	m.Answer[0].Header().Ttl = 0
	again, _ := s.Lookup(q)
	if again.Answer[0].Header().Ttl != 60 {
		t.Fatal("response mutated snapshot")
	}
}
