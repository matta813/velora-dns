package config

import (
	"os"
	"testing"
	"time"
)

func TestLANExampleParses(t *testing.T) {
	data, err := os.ReadFile("../../configs/config.lan.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(data, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if len(c.DNS.Listen) != 2 || c.DNS.Listen[0] != "0.0.0.0:53" || c.DNS.Listen[1] != "[::]:53" || c.HTTP.Listen != "127.0.0.1:8080" {
		t.Fatalf("unsafe or incomplete LAN example: %+v", c)
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

func TestEncryptedDNSConfiguration(t *testing.T) {
	env := map[string]string{"VELORA_DNS_DOT_LISTEN": "127.0.0.1:853", "VELORA_DNS_DOH_LISTEN": "127.0.0.1:8443", "VELORA_DNS_DOQ_LISTEN": "127.0.0.1:8853", "VELORA_DNS_TLS_CERT_FILE": "/cert.pem", "VELORA_DNS_TLS_KEY_FILE": "/key.pem", "VELORA_DNS_UPSTREAMS": "tls://1.1.1.1:853,https://1.1.1.1:443/dns-query"}
	c, err := Parse(nil, func(key string) (string, bool) { value, ok := env[key]; return value, ok })
	if err != nil || c.DNS.DoTListen == "" || c.DNS.DoHListen == "" || c.DNS.DoQListen != "127.0.0.1:8853" || len(c.DNS.Upstreams) != 2 {
		t.Fatalf("encrypted DNS config: %+v %v", c.DNS, err)
	}
	if _, err = Parse([]byte("dns:\n  dot_listen: 127.0.0.1:853\n"), func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("DoT without certificate accepted")
	}
	env["VELORA_DNS_DOQ_LISTEN"] = "not-an-address"
	if _, err = Parse(nil, func(key string) (string, bool) { value, ok := env[key]; return value, ok }); err == nil {
		t.Fatal("invalid DoQ environment listener accepted")
	}
}

func TestDNSSECConfiguration(t *testing.T) {
	anchor := ". 3600 IN DS 20326 8 2 E06D44B80B8F1D39A95C0B0D7C65D08458E880409BBC683457104237C7F8EC8D"
	env := map[string]string{"VELORA_DNS_DNSSEC": "true", "VELORA_DNS_TRUST_ANCHORS": anchor}
	c, err := Parse(nil, func(key string) (string, bool) { value, ok := env[key]; return value, ok })
	if err != nil || !c.DNS.DNSSEC || len(c.DNS.TrustAnchors) != 1 {
		t.Fatalf("DNSSEC config: %+v %v", c.DNS, err)
	}
	if _, err = Parse([]byte("dns:\n  dnssec: true\n"), func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("DNSSEC without trust anchor accepted")
	}
}
