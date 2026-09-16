package zones

import (
	"strings"
	"testing"
)

func TestZoneFileRoundTrip(t *testing.T) {
	input := `$ORIGIN example.test.
@ 3600 IN SOA ns.example.test. hostmaster.example.test. 42 3600 600 86400 300
@ 300 IN NS ns.example.test.
www 60 IN A 192.0.2.8
txt 60 IN TXT "hello world"
`
	z, err := ParseZoneFile(input)
	if err != nil || z.Name != "example.test." || len(z.Records) != 3 {
		t.Fatalf("parse: %+v %v", z, err)
	}
	exported, err := FormatZoneFile(z)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseZoneFile(exported)
	if err != nil || again.Name != z.Name || len(again.Records) != len(z.Records) {
		t.Fatalf("round trip: %+v %v", again, err)
	}
}

func TestZoneFileRejectsUnsupportedOrMissingSOA(t *testing.T) {
	for _, input := range []string{"www 60 IN A 192.0.2.1", "@ 60 IN SOA ns.test. hostmaster.test. 1 1 1 1 1\n@ 60 IN CAA 0 issue \"ca.example\""} {
		if _, err := ParseZoneFile(input); err == nil {
			t.Fatalf("accepted invalid input %q", input)
		}
	}
	if _, err := ReadZoneFile(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("accepted oversized input")
	}
}
