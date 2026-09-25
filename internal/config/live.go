package config

import (
	"fmt"
	"reflect"
	"strings"
)

// RestartRequiredError lists configuration fields that the running process
// cannot activate safely without replacing a listener or rebuilding a service.
type RestartRequiredError struct{ Fields []string }

// Clone separates mutable slices before JSON decoding a candidate config.
func (c Config) Clone() Config {
	c.DNS.Listen = append([]string(nil), c.DNS.Listen...)
	c.DNS.Upstreams = append([]string(nil), c.DNS.Upstreams...)
	c.DNS.AllowedClients = append([]string(nil), c.DNS.AllowedClients...)
	c.DNS.TrustAnchors = append([]string(nil), c.DNS.TrustAnchors...)
	c.HTTP.AllowedHosts = append([]string(nil), c.HTTP.AllowedHosts...)
	c.Filtering.Blocklist = append([]string(nil), c.Filtering.Blocklist...)
	c.Filtering.Allowlist = append([]string(nil), c.Filtering.Allowlist...)
	c.TSIG.Keys = append([]TSIGKey(nil), c.TSIG.Keys...)
	c.Transfers = append([]Transfer(nil), c.Transfers...)
	c.Cluster.ReplicaDSNs = append([]string(nil), c.Cluster.ReplicaDSNs...)
	return c
}

func (e *RestartRequiredError) Error() string {
	return fmt.Sprintf("configuration requires a restart for: %s", strings.Join(e.Fields, ", "))
}

func RestartRequiredFields(previous, next Config) []string {
	var fields []string
	compare := func(prefix string, before, after any, live map[string]bool) {
		oldValue, newValue := reflect.ValueOf(before), reflect.ValueOf(after)
		kind := oldValue.Type()
		for i := 0; i < kind.NumField(); i++ {
			name := strings.Split(kind.Field(i).Tag.Get("json"), ",")[0]
			if name == "-" || live[name] {
				continue
			}
			if !reflect.DeepEqual(oldValue.Field(i).Interface(), newValue.Field(i).Interface()) {
				fields = append(fields, prefix+name)
			}
		}
	}
	compare("dns.", previous.DNS, next.DNS, map[string]bool{"allowed_clients": true, "max_concurrent": true, "global_qps": true, "client_qps": true, "rate_limit_burst": true, "rate_limit_enabled": true})
	compare("cache.", previous.Cache, next.Cache, map[string]bool{"upstream_ttl": true})
	compare("http.", previous.HTTP, next.HTTP, map[string]bool{"allowed_hosts": true, "web_dir": true})
	compare("filtering.", previous.Filtering, next.Filtering, map[string]bool{"block_mode": true})
	compare("query_log.", previous.QueryLog, next.QueryLog, map[string]bool{"enabled": true})
	compare("tsig.", previous.TSIG, next.TSIG, nil)
	if !reflect.DeepEqual(previous.Transfers, next.Transfers) {
		fields = append(fields, "transfers")
	}
	compare("dhcp.", previous.DHCP, next.DHCP, nil)
	compare("cluster.", previous.Cluster, next.Cluster, nil)
	compare("node.", previous.Node, next.Node, nil)
	if previous.LogLevel != next.LogLevel {
		fields = append(fields, "log_level")
	}
	return fields
}
