package config

import (
	"testing"
	"time"
)

func TestParsing(t *testing.T) {
	lookup := func(k string) (string, bool) {
		v, ok := map[string]string{"VELORA_DNS_LISTEN": "127.0.0.1:5454", "VELORA_CACHE_MAX_ENTRIES": "5"}[k]
		return v, ok
	}
	c, err := Parse([]byte("dns:\n  timeout: 500ms\ncache:\n  max_entries: 10\n"), lookup)
	if err != nil {
		t.Fatal(err)
	}
	if c.DNS.Listen[0] != "127.0.0.1:5454" || c.DNS.Timeout != 500*time.Millisecond || c.Cache.MaxEntries != 5 {
		t.Fatalf("incorrect overrides: %+v", c)
	}
}
func TestRejectInvalid(t *testing.T) {
	for _, data := range []string{"typo: 1", "dns:\n  allowed_clients: []", "dns:\n  retries: -1", "cache:\n  max_entries: -2", "dns:\n  timeout: 0s", "dns:\n  upstreams: [localhost:53]", "---\nlog_level: info\n---\nlog_level: debug", "http:\n  listen: 127.0.0.1:0"} {
		t.Run(data, func(t *testing.T) {
			if _, err := Parse([]byte(data), func(string) (string, bool) { return "", false }); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
	_, err := Parse(nil, func(k string) (string, bool) { return "bad", k == "VELORA_DNS_RETRIES" })
	if err == nil {
		t.Fatal("accepted invalid environment")
	}
}
