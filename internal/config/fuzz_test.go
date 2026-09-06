package config

import "testing"

func FuzzParse(f *testing.F) {
	for _, seed := range []string{"dns:\n  listen: [127.0.0.1:5353]", "http:\n  allowed_hosts: ['*']", "a: &a [*a]", "cache:\n  max_entries: -1", "---\n---"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			t.Skip()
		}
		_, _ = Parse(data, func(string) (string, bool) { return "", false })
	})
}
