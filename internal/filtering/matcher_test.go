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
