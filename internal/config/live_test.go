package config

import (
	"reflect"
	"testing"
)

func TestRestartRequiredFields(t *testing.T) {
	previous := Default()
	next := previous
	next.DNS.AllowedClients = []string{"192.0.2.0/24"}
	next.Cache.UpstreamTTL = 120
	next.HTTP.AllowedHosts = []string{"localhost"}
	next.QueryLog.Enabled = true
	if fields := RestartRequiredFields(previous, next); len(fields) != 0 {
		t.Fatalf("live fields rejected: %v", fields)
	}
	next.DNS.Listen = []string{"127.0.0.1:5353"}
	next.DNS.Upstreams = []string{"192.0.2.1:53"}
	next.HTTP.Listen = "127.0.0.1:9090"
	next.LogLevel = "debug"
	if fields := RestartRequiredFields(previous, next); !reflect.DeepEqual(fields, []string{"dns.listen", "dns.upstreams", "http.listen", "log_level"}) {
		t.Fatalf("restart fields: %v", fields)
	}
}
