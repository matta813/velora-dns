package filtering

import "testing"

func TestMatcherAllowOverridesAndWildcardBoundaries(t *testing.T) {
	m, err := New([]Rule{{Domain: "ads.example", Wildcard: true, Action: Block}, {Domain: "safe.ads.example", Action: Allow}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		action Action
		ok     bool
	}{{"ads.example.", Block, true}, {"a.ads.example", Block, true}, {"safe.ads.example", Allow, true}, {"notads.example", Allow, false}} {
		got, ok := m.Match(tc.name)
		if got != tc.action || ok != tc.ok {
			t.Errorf("%s: %v,%v", tc.name, got, ok)
		}
	}
}

func TestMatcherUnderscoreDomains(t *testing.T) {
	m, err := New([]Rule{
		{Domain: "_dmarc.example.com", Wildcard: true, Action: Block},
		{Domain: "_domainkey.example.com", Wildcard: true, Action: Block},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		blocked bool
	}{{"_dmarc.example.com", true}, {"_domainkey.example.com", true}, {"mail._domainkey.example.com", true}, {"safe.example.com", false}} {
		if got := m.Blocked(tc.name); got != tc.blocked {
			t.Errorf("%s: blocked=%v, want %v", tc.name, got, tc.blocked)
		}
	}
}
