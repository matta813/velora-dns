package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAuthEnvironmentAndSecretRedaction(t *testing.T) {
	env := map[string]string{"VELORA_BOOTSTRAP_USERNAME": "root.admin", "VELORA_BOOTSTRAP_PASSWORD": "correct horse battery staple", "VELORA_AUTH_SESSION_TTL": "2h", "VELORA_AUTH_SECURE_COOKIES": "true"}
	c, err := Parse(nil, func(key string) (string, bool) { value, ok := env[key]; return value, ok })
	if err != nil || c.Auth.BootstrapUsername != "root.admin" || c.Auth.SessionTTL != 2*time.Hour || !c.Auth.SecureCookies {
		t.Fatalf("auth config: %+v %v", c.Auth, err)
	}
	encoded, err := json.Marshal(c)
	if err != nil || strings.Contains(string(encoded), c.Auth.BootstrapPassword) {
		t.Fatal("bootstrap password exposed by config JSON")
	}
	for _, value := range []string{"short", strings.Repeat("x", 1025)} {
		env["VELORA_BOOTSTRAP_PASSWORD"] = value
		if _, err = Parse(nil, func(key string) (string, bool) { item, ok := env[key]; return item, ok }); err == nil {
			t.Fatal("invalid bootstrap password accepted")
		}
	}
}

func TestQueryLogEnvironmentAndBounds(t *testing.T) {
	env := map[string]string{"VELORA_QUERY_LOG_ENABLED": "true", "VELORA_QUERY_LOG_RETENTION": "24h", "VELORA_QUERY_LOG_QUEUE_SIZE": "32", "VELORA_QUERY_LOG_MAX_ROWS": "250"}
	c, err := Parse(nil, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil || !c.QueryLog.Enabled || c.QueryLog.Retention != 24*time.Hour || c.QueryLog.MaxRows != 250 || c.QueryLog.QueueSize != 32 {
		t.Fatalf("%+v %v", c.QueryLog, err)
	}
	for _, v := range []string{"0s", "-1h", "oops"} {
		env["VELORA_QUERY_LOG_RETENTION"] = v
		if _, err = Parse(nil, func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err == nil {
			t.Fatal("invalid retention accepted")
		}
	}
}

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
func TestFilteringListsFromYAMLAndEnvironment(t *testing.T) {
	lookup := func(k string) (string, bool) {
		return "ads.example, telemetry.example", k == "VELORA_FILTERING_BLOCKLIST"
	}
	c, err := Parse([]byte("filtering:\n  allowlist: [safe.ads.example]\n"), lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Filtering.Blocklist) != 2 || c.Filtering.Allowlist[0] != "safe.ads.example" {
		t.Fatalf("unexpected filtering config: %#v", c.Filtering)
	}
}
func TestBlockModeValidation(t *testing.T) {
	for _, mode := range []string{"", "NXDOMAIN", "ZERO"} {
		c, err := Parse([]byte("filtering:\n  block_mode: "+mode+"\n"), func(string) (string, bool) { return "", false })
		if err != nil {
			t.Errorf("block_mode %q rejected: %v", mode, err)
		}
		_ = c
	}
	if _, err := Parse([]byte("filtering:\n  block_mode: BOUNCE\n"), func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("accepted invalid block_mode")
	}
}
