package config

import (
	"errors"
	"strings"
	"testing"
)

func fieldsOf(t *testing.T, c Config) map[string]string {
	t.Helper()
	err := c.Validate()
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	out := map[string]string{}
	for _, field := range invalid.Fields {
		out[field.Field] = field.Message
	}
	return out
}

func TestValidateReportsEveryFieldAtOnce(t *testing.T) {
	c := Default()
	c.DNS.AllowedClients = []string{"127.0.0.0/8", "not-a-cidr"}
	c.DNS.Upstreams = []string{"1.1.1.1:53", "dns.example:53"}
	c.DNS.Listen = []string{"127.0.0.1:5353", "localhost:53"}
	c.DNS.Retries = 9
	c.Cache.MaxEntries = -1
	c.Filtering.Blocklist = []string{"ads.example", "bad domain"}
	c.LogLevel = "loud"
	c.HTTP.WebDir = "../../etc"
	fields := fieldsOf(t, c)
	for _, field := range []string{"dns.allowed_clients[1]", "dns.upstreams[1]", "dns.listen[1]", "dns.retries", "cache.max_entries", "filtering.blocklist[1]", "log_level", "http.web_dir"} {
		if fields[field] == "" {
			t.Errorf("missing error for %s in %v", field, fields)
		}
	}
	if _, ok := fields["dns.allowed_clients[0]"]; ok {
		t.Fatal("valid entries must not be reported")
	}
	if err := c.Validate(); !strings.Contains(err.Error(), "dns.retries: must be 0–3") {
		t.Fatalf("message: %v", err)
	}
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config must be valid: %v", err)
	}
}

func TestValidateDetectsListenerConflicts(t *testing.T) {
	c := Default()
	c.DNS.Listen = []string{"0.0.0.0:53"}
	c.HTTP.Listen = "192.168.1.2:53" // TCP/53 overlaps the wildcard DNS listener
	fields := fieldsOf(t, c)
	if !strings.Contains(fields["http.listen"], "tcp/53 conflicts with dns.listen[0]") {
		t.Fatalf("conflict: %v", fields)
	}
	c = Default()
	c.DNS.Listen = []string{"127.0.0.1:53"}
	c.DNS.TLSCertFile, c.DNS.TLSKeyFile = "/etc/velora/cert.pem", "/etc/velora/key.pem"
	c.DNS.DoQListen = "127.0.0.1:53" // UDP/53 again
	c.HTTP.Listen = "127.0.0.1:8080"
	if fields = fieldsOf(t, c); !strings.Contains(fields["dns.doq_listen"], "udp/53") {
		t.Fatalf("doq conflict: %v", fields)
	}
	// Different families or protocols do not conflict.
	c = Default()
	c.DNS.Listen = []string{"0.0.0.0:53", "[::]:53"}
	c.DNS.TLSCertFile, c.DNS.TLSKeyFile = "/etc/velora/cert.pem", "/etc/velora/key.pem"
	c.DNS.DoTListen = "0.0.0.0:853"
	c.DNS.DoQListen = "0.0.0.0:853"
	if err := c.Validate(); err != nil {
		t.Fatalf("no conflict expected: %v", err)
	}
}
