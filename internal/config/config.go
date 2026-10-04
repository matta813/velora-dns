// Package config loads strict YAML configuration with explicit environment overrides.
package config

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	wire "github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

type DNS struct {
	Listen           []string      `yaml:"listen" json:"listen"`
	Upstreams        []string      `yaml:"upstreams" json:"upstreams"`
	AllowedClients   []string      `yaml:"allowed_clients" json:"allowed_clients"`
	Timeout          time.Duration `yaml:"timeout" json:"timeout"`
	Retries          int           `yaml:"retries" json:"retries"`
	MaxConcurrent    int           `yaml:"max_concurrent" json:"max_concurrent"`
	GlobalQPS        int           `yaml:"global_qps" json:"global_qps"`
	ClientQPS        int           `yaml:"client_qps" json:"client_qps"`
	RateLimitBurst   int           `yaml:"rate_limit_burst" json:"rate_limit_burst"`
	RateLimitEnabled bool          `yaml:"rate_limit_enabled" json:"rate_limit_enabled"`
	MaxTCPConns      int           `yaml:"max_tcp_connections" json:"max_tcp_connections"`
	DoTListen        string        `yaml:"dot_listen" json:"dot_listen"`
	DoHListen        string        `yaml:"doh_listen" json:"doh_listen"`
	DoQListen        string        `yaml:"doq_listen" json:"doq_listen"`
	TLSCertFile      string        `yaml:"tls_cert_file" json:"tls_cert_file"`
	TLSKeyFile       string        `yaml:"tls_key_file" json:"-"`
	DNSSEC           bool          `yaml:"dnssec" json:"dnssec"`
	TrustAnchors     []string      `yaml:"trust_anchors" json:"trust_anchors"`
}
type Cache struct {
	MaxEntries  int `yaml:"max_entries" json:"max_entries"`
	UpstreamTTL int `yaml:"upstream_ttl" json:"upstream_ttl"`
}
type HTTP struct {
	AllowedHosts []string `yaml:"allowed_hosts" json:"allowed_hosts"`
	Listen       string   `yaml:"listen" json:"listen"`
	WebDir       string   `yaml:"web_dir" json:"web_dir"`
}
type Filtering struct {
	BlockMode string   `yaml:"block_mode" json:"block_mode"`
	Blocklist []string `yaml:"blocklist" json:"blocklist"`
	Allowlist []string `yaml:"allowlist" json:"allowlist"`
}
type QueryLog struct {
	MaxRows   int           `yaml:"max_rows" json:"max_rows"`
	Enabled   bool          `yaml:"enabled" json:"enabled"`
	QueueSize int           `yaml:"queue_size" json:"queue_size"`
	Retention time.Duration `yaml:"retention" json:"retention"`
}
type Management struct {
	BootstrapUsername string `yaml:"-" json:"-"`
	BootstrapPassword string `yaml:"-" json:"-"`
}
type TSIGKey struct {
	Name      string `yaml:"name" json:"name"`
	Algorithm string `yaml:"algorithm" json:"algorithm"`
	Secret    string `yaml:"secret" json:"-"`
}

type TSIG struct {
	Keys []TSIGKey `yaml:"keys" json:"keys"`
}

type Transfer struct {
	Zone     string `yaml:"zone" json:"zone"`
	Primary  string `yaml:"primary" json:"primary"`
	TSIGKey  string `yaml:"tsig_key" json:"tsig_key"`
	Interval int    `yaml:"interval" json:"interval"`
}

type DHCP struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	Listen     string `yaml:"listen" json:"listen"`
	PublishDNS bool   `yaml:"publish_dns" json:"publish_dns"`
}

type Cluster struct {
	Enabled         bool          `yaml:"enabled" json:"enabled"`
	PrimaryDSN      string        `yaml:"primary_dsn" json:"-"`
	ReplicaDSNs     []string      `yaml:"replica_dsns" json:"-"`
	MaxOpenConns    int           `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
}

type Node struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Address string `yaml:"address" json:"address"`
}

type Config struct {
	DNS            DNS        `yaml:"dns" json:"dns"`
	Cache          Cache      `yaml:"cache" json:"cache"`
	HTTP           HTTP       `yaml:"http" json:"http"`
	Filtering      Filtering  `yaml:"filtering" json:"filtering"`
	QueryLog       QueryLog   `yaml:"query_log" json:"query_log"`
	TSIG           TSIG       `yaml:"tsig" json:"tsig"`
	Transfers      []Transfer `yaml:"transfers" json:"transfers"`
	DHCP           DHCP       `yaml:"dhcp" json:"dhcp"`
	Cluster        Cluster    `yaml:"cluster" json:"cluster"`
	Node           Node       `yaml:"node" json:"node"`
	Management     Management `yaml:"-" json:"-"`
	DatabasePath   string     `yaml:"database_path" json:"-"`
	DatabaseDriver string     `yaml:"database_driver" json:"-"`
	DatabaseURL    string     `yaml:"database_url" json:"-"`
	LogLevel       string     `yaml:"log_level" json:"log_level"`
}

func Default() Config {
	return Config{DNS: DNS{Listen: []string{"127.0.0.1:3535"}, Upstreams: []string{"1.1.1.1:53", "9.9.9.9:53"}, AllowedClients: []string{"127.0.0.0/8", "::1/128"}, Timeout: 2 * time.Second, Retries: 1, MaxConcurrent: 256, GlobalQPS: 1000, ClientQPS: 100, RateLimitBurst: 100, RateLimitEnabled: false, MaxTCPConns: 256}, Cache: Cache{MaxEntries: 10000, UpstreamTTL: 86400}, HTTP: HTTP{Listen: "127.0.0.1:8080", WebDir: "web/dist", AllowedHosts: []string{"localhost", "127.0.0.1", "::1"}}, QueryLog: QueryLog{Enabled: false, MaxRows: 100000, QueueSize: 1024, Retention: 7 * 24 * time.Hour}, Cluster: Cluster{MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: 5 * time.Minute}, Node: Node{ID: "node-1", Name: "Primary"}, DatabasePath: "data/velora.db", DatabaseDriver: "sqlite", LogLevel: "info"}
}
func Load(path string) (Config, error) {
	var data []byte
	if path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}
	return Parse(data, os.LookupEnv)
}
func Parse(data []byte, lookup func(string) (string, bool)) (Config, error) {
	c := Default()
	if len(data) > 0 {
		d := yaml.NewDecoder(bytes.NewReader(data))
		d.KnownFields(true)
		if err := d.Decode(&c); err != nil {
			return c, fmt.Errorf("decode config: %w", err)
		}
		var extra any
		if err := d.Decode(&extra); err != io.EOF {
			return c, fmt.Errorf("config must contain one YAML document")
		}
	}
	for key, target := range map[string]*string{"HTTP_LISTEN": &c.HTTP.Listen, "WEB_DIR": &c.HTTP.WebDir, "DATABASE_PATH": &c.DatabasePath, "DATABASE_DRIVER": &c.DatabaseDriver, "DATABASE_URL": &c.DatabaseURL, "LOG_LEVEL": &c.LogLevel, "FILTERING_BLOCK_MODE": &c.Filtering.BlockMode, "DNS_DOT_LISTEN": &c.DNS.DoTListen, "DNS_DOH_LISTEN": &c.DNS.DoHListen, "DNS_DOQ_LISTEN": &c.DNS.DoQListen, "DNS_TLS_CERT_FILE": &c.DNS.TLSCertFile, "DNS_TLS_KEY_FILE": &c.DNS.TLSKeyFile, "DHCP_LISTEN": &c.DHCP.Listen} {
		if v, ok := lookup("VELORA_" + key); ok {
			*target = v
		}
	}
	if v, ok := lookup("VELORA_BOOTSTRAP_USERNAME"); ok {
		c.Management.BootstrapUsername = v
	}
	if v, ok := lookup("VELORA_BOOTSTRAP_PASSWORD"); ok {
		c.Management.BootstrapPassword = v
	}
	for key, target := range map[string]*[]string{"HTTP_ALLOWED_HOSTS": &c.HTTP.AllowedHosts, "DNS_LISTEN": &c.DNS.Listen, "DNS_UPSTREAMS": &c.DNS.Upstreams, "DNS_ALLOWED_CLIENTS": &c.DNS.AllowedClients, "DNS_TRUST_ANCHORS": &c.DNS.TrustAnchors} {
		if v, ok := lookup("VELORA_" + key); ok {
			*target = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' })
			for i := range *target {
				(*target)[i] = strings.TrimSpace((*target)[i])
			}
		}
	}
	for key, target := range map[string]*[]string{"FILTERING_BLOCKLIST": &c.Filtering.Blocklist, "FILTERING_ALLOWLIST": &c.Filtering.Allowlist} {
		if v, ok := lookup("VELORA_" + key); ok {
			*target = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' })
			for i := range *target {
				(*target)[i] = strings.TrimSpace((*target)[i])
			}
		}
	}
	for key, target := range map[string]*int{"DNS_RETRIES": &c.DNS.Retries, "DNS_MAX_CONCURRENT": &c.DNS.MaxConcurrent, "DNS_GLOBAL_QPS": &c.DNS.GlobalQPS, "DNS_CLIENT_QPS": &c.DNS.ClientQPS, "DNS_RATE_LIMIT_BURST": &c.DNS.RateLimitBurst, "DNS_MAX_TCP_CONNECTIONS": &c.DNS.MaxTCPConns, "CACHE_MAX_ENTRIES": &c.Cache.MaxEntries} {
		if v, ok := lookup("VELORA_" + key); ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return c, fmt.Errorf("invalid VELORA_%s", key)
			}
			*target = n
		}
	}
	if v, ok := lookup("VELORA_CACHE_UPSTREAM_TTL"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return c, fmt.Errorf("invalid VELORA_CACHE_UPSTREAM_TTL")
		}
		c.Cache.UpstreamTTL = int(n)
	}
	if v, ok := lookup("VELORA_DNS_TIMEOUT"); ok {
		n, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("invalid VELORA_DNS_TIMEOUT")
		}
		c.DNS.Timeout = n
	}
	if v, ok := lookup("VELORA_QUERY_LOG_ENABLED"); ok {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("invalid VELORA_QUERY_LOG_ENABLED")
		}
		c.QueryLog.Enabled = enabled
	}
	if v, ok := lookup("VELORA_DNS_DNSSEC"); ok {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("invalid VELORA_DNS_DNSSEC")
		}
		c.DNS.DNSSEC = enabled
	}
	if v, ok := lookup("VELORA_DHCP_ENABLED"); ok {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("invalid VELORA_DHCP_ENABLED")
		}
		c.DHCP.Enabled = enabled
	}
	if v, ok := lookup("VELORA_DHCP_PUBLISH_DNS"); ok {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("invalid VELORA_DHCP_PUBLISH_DNS")
		}
		c.DHCP.PublishDNS = enabled
	}
	if v, ok := lookup("VELORA_HA_ENABLED"); ok {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("invalid VELORA_HA_ENABLED")
		}
		c.Cluster.Enabled = enabled
	}
	if v, ok := lookup("VELORA_DNS_RATE_LIMIT_ENABLED"); ok {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("invalid VELORA_DNS_RATE_LIMIT_ENABLED")
		}
		c.DNS.RateLimitEnabled = enabled
	}
	for key, target := range map[string]*int{"QUERY_LOG_QUEUE_SIZE": &c.QueryLog.QueueSize, "QUERY_LOG_MAX_ROWS": &c.QueryLog.MaxRows} {
		if v, ok := lookup("VELORA_" + key); ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return c, fmt.Errorf("invalid VELORA_%s", key)
			}
			*target = n
		}
	}
	if v, ok := lookup("VELORA_QUERY_LOG_RETENTION"); ok {
		duration, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("invalid VELORA_QUERY_LOG_RETENTION")
		}
		c.QueryLog.Retention = duration
	}
	return c, c.Validate()
}

// FieldError names one invalid configuration field by its YAML/JSON path,
// for example "dns.allowed_clients[1]".
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError lists every problem found in a candidate configuration.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, field := range e.Fields {
		parts = append(parts, field.Field+": "+field.Message)
	}
	return strings.Join(parts, "; ")
}

// Validate checks the whole configuration and reports all problems at once
// as a *ValidationError, so the API and UI can point at each field.
func (c Config) Validate() error {
	var errs []FieldError
	add := func(field, format string, args ...any) {
		errs = append(errs, FieldError{Field: field, Message: fmt.Sprintf(format, args...)})
	}
	check := func(field string, err error) {
		if err != nil {
			add(field, "%s", err.Error())
		}
	}
	if (c.Management.BootstrapUsername == "") != (c.Management.BootstrapPassword == "") || (c.Management.BootstrapPassword != "" && len(c.Management.BootstrapPassword) < 12) {
		add("management.bootstrap_password", "bootstrap username and password must both be set; password requires at least 12 characters")
	}
	if c.Filtering.BlockMode != "" && c.Filtering.BlockMode != "NXDOMAIN" && c.Filtering.BlockMode != "ZERO" {
		add("filtering.block_mode", "must be NXDOMAIN or ZERO")
	}
	for list, domains := range map[string][]string{"filtering.blocklist": c.Filtering.Blocklist, "filtering.allowlist": c.Filtering.Allowlist} {
		for i, domain := range domains {
			if !validDomain(domain) {
				add(fmt.Sprintf("%s[%d]", list, i), "%q is not a valid domain", domain)
			}
		}
	}
	if len(c.DNS.Listen) == 0 || len(c.DNS.Listen) > 8 {
		add("dns.listen", "DNS requires 1–8 listeners")
	}
	if len(c.DNS.Upstreams) == 0 || len(c.DNS.Upstreams) > 8 {
		add("dns.upstreams", "DNS requires 1–8 upstreams")
	}
	if (c.DNS.TLSCertFile == "") != (c.DNS.TLSKeyFile == "") {
		add("dns.tls_key_file", "dns.tls_cert_file and dns.tls_key_file must both be set")
	}
	for field, value := range map[string]string{"dns.tls_cert_file": c.DNS.TLSCertFile, "dns.tls_key_file": c.DNS.TLSKeyFile} {
		if value != "" {
			check(field, safePath(value))
		}
	}
	if c.DNS.DNSSEC && len(c.DNS.TrustAnchors) == 0 {
		add("dns.trust_anchors", "required when DNSSEC validation is enabled")
	}
	if c.DHCP.Enabled {
		if c.DHCP.Listen == "" {
			c.DHCP.Listen = "0.0.0.0:67"
		}
		check("dhcp.listen", address(c.DHCP.Listen, true))
	}
	for i, anchor := range c.DNS.TrustAnchors {
		rr, err := wire.NewRR(anchor)
		if err != nil || rr == nil || rr.Header().Rrtype != wire.TypeDS {
			add(fmt.Sprintf("dns.trust_anchors[%d]", i), "invalid DNSSEC DS trust anchor %q", anchor)
		}
	}
	for _, item := range []struct{ name, field, value string }{{"TLS", "dns.dot_listen", c.DNS.DoTListen}, {"HTTPS", "dns.doh_listen", c.DNS.DoHListen}, {"QUIC", "dns.doq_listen", c.DNS.DoQListen}} {
		if item.value != "" {
			if c.DNS.TLSCertFile == "" {
				add(item.field, "DNS-over-%s requires a TLS certificate and key", item.name)
			}
			check(item.field, address(item.value, true))
		}
	}
	seen := map[string]bool{}
	for i, a := range c.DNS.Listen {
		field := fmt.Sprintf("dns.listen[%d]", i)
		if err := address(a, true); err != nil {
			check(field, err)
			continue
		}
		if seen[a] {
			add(field, "duplicate listener %q", a)
		}
		seen[a] = true
	}
	for i, a := range c.DNS.Upstreams {
		field := fmt.Sprintf("dns.upstreams[%d]", i)
		if err := address(a, false); err != nil {
			check(field, err)
			continue
		}
		if seen[a] {
			add(field, "upstream %q is one of this server's own listeners", a)
		}
	}
	check("http.listen", address(c.HTTP.Listen, true))
	for _, conflict := range c.listenerConflicts() {
		add(conflict.Field, "%s", conflict.Message)
	}
	if len(c.HTTP.AllowedHosts) == 0 {
		add("http.allowed_hosts", "cannot be empty")
	}
	for i, host := range c.HTTP.AllowedHosts {
		if !validHost(host) {
			add(fmt.Sprintf("http.allowed_hosts[%d]", i), "invalid host %q", host)
		}
	}
	if len(c.DNS.AllowedClients) == 0 {
		add("dns.allowed_clients", "cannot be empty")
	}
	for i, v := range c.DNS.AllowedClients {
		if prefix, err := netip.ParsePrefix(v); err != nil || prefix.Addr().Zone() != "" {
			add(fmt.Sprintf("dns.allowed_clients[%d]", i), "invalid client network %q; use CIDR notation such as 192.168.1.0/24", v)
		}
	}
	if c.DNS.Timeout < 10*time.Millisecond || c.DNS.Timeout > 10*time.Second {
		add("dns.timeout", "must be 10ms–10s")
	}
	if c.DNS.Retries < 0 || c.DNS.Retries > 3 {
		add("dns.retries", "must be 0–3")
	}
	for _, limit := range []struct {
		field    string
		value    int
		min, max int
	}{
		{"cache.max_entries", c.Cache.MaxEntries, 0, 1000000},
		{"dns.max_concurrent", c.DNS.MaxConcurrent, 1, 10000},
		{"dns.global_qps", c.DNS.GlobalQPS, 1, 100000},
		{"dns.client_qps", c.DNS.ClientQPS, 1, 100000},
		{"dns.rate_limit_burst", c.DNS.RateLimitBurst, 1, 100000},
		{"dns.max_tcp_connections", c.DNS.MaxTCPConns, 1, 100000},
		{"cache.upstream_ttl", c.Cache.UpstreamTTL, 0, 604800},
		{"query_log.queue_size", c.QueryLog.QueueSize, 1, 100000},
		{"query_log.max_rows", c.QueryLog.MaxRows, 1, 1000000},
	} {
		if limit.value < limit.min || limit.value > limit.max {
			add(limit.field, "must be %d–%d", limit.min, limit.max)
		}
	}
	if c.QueryLog.Retention < time.Minute || c.QueryLog.Retention > 365*24*time.Hour {
		add("query_log.retention", "must be 1 minute to 365 days")
	}
	if c.DatabaseDriver == "" {
		c.DatabaseDriver = "sqlite"
	}
	if c.DatabaseDriver != "sqlite" && c.DatabaseDriver != "postgres" {
		add("database_driver", "must be sqlite or postgres")
	}
	if c.DatabaseDriver == "postgres" && c.DatabaseURL == "" {
		add("database_url", "required when database_driver is postgres")
	}
	if c.DatabaseDriver == "sqlite" {
		if c.DatabasePath == "" {
			add("database_path", "cannot be empty for sqlite")
		} else {
			check("database_path", safePath(c.DatabasePath))
		}
	}
	if c.HTTP.WebDir == "" {
		add("http.web_dir", "cannot be empty")
	} else {
		check("http.web_dir", safePath(c.HTTP.WebDir))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		add("log_level", "must be debug, info, warn or error")
	}
	if len(errs) == 0 {
		return nil
	}
	return &ValidationError{Fields: errs}
}

// safePath rejects values that are not plain filesystem paths.
func safePath(value string) error {
	if strings.ContainsAny(value, "\x00\r\n") || strings.Contains(value, "://") || len(value) > 4096 {
		return fmt.Errorf("must be a plain filesystem path")
	}
	for _, segment := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return fmt.Errorf("must not contain .. segments")
		}
	}
	return nil
}

func validDomain(value string) bool {
	value = strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "*."), ".")
	if value == "" || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
				return false
			}
		}
	}
	return true
}

type socket struct {
	field, network string
	addr           netip.AddrPort
}

// listenerConflicts finds listeners that would bind the same port and
// protocol, including a wildcard address overlapping a specific one.
func (c Config) listenerConflicts() []FieldError {
	var sockets []socket
	addSocket := func(field, value string, networks ...string) {
		addr, err := netip.ParseAddrPort(value)
		if err != nil {
			return
		}
		for _, network := range networks {
			sockets = append(sockets, socket{field: field, network: network, addr: netip.AddrPortFrom(addr.Addr().Unmap(), addr.Port())})
		}
	}
	for i, value := range c.DNS.Listen {
		addSocket(fmt.Sprintf("dns.listen[%d]", i), value, "udp", "tcp")
	}
	addSocket("dns.dot_listen", c.DNS.DoTListen, "tcp")
	addSocket("dns.doh_listen", c.DNS.DoHListen, "tcp")
	addSocket("dns.doq_listen", c.DNS.DoQListen, "udp")
	addSocket("http.listen", c.HTTP.Listen, "tcp")
	if c.DHCP.Enabled {
		listen := c.DHCP.Listen
		if listen == "" {
			listen = "0.0.0.0:67"
		}
		addSocket("dhcp.listen", listen, "udp")
	}
	var out []FieldError
	reported := map[string]bool{}
	for i, a := range sockets {
		for _, b := range sockets[:i] {
			if a.field == b.field || a.network != b.network || a.addr.Port() != b.addr.Port() || reported[a.field+b.field] {
				continue
			}
			sameFamily := a.addr.Addr().Is4() == b.addr.Addr().Is4()
			overlap := a.addr.Addr() == b.addr.Addr() || (sameFamily && (a.addr.Addr().IsUnspecified() || b.addr.Addr().IsUnspecified()))
			if overlap {
				reported[a.field+b.field] = true
				out = append(out, FieldError{Field: a.field, Message: fmt.Sprintf("%s/%d conflicts with %s (%s)", a.network, a.addr.Port(), b.field, b.addr)})
			}
		}
	}
	return out
}

func address(a string, listen bool) error {
	if !listen {
		if strings.HasPrefix(a, "https://") {
			u, err := url.Parse(a)
			if err != nil || u.Scheme != "https" || u.Path != "/dns-query" || u.RawQuery != "" || u.Fragment != "" {
				return fmt.Errorf("invalid DoH upstream %q", a)
			}
			host, port, err := net.SplitHostPort(u.Host)
			if err != nil || net.ParseIP(host) == nil || port == "" {
				return fmt.Errorf("DoH upstream requires IP literal, port and /dns-query path")
			}
			return nil
		}
		a = strings.TrimPrefix(a, "tls://")
	}
	host, port, err := net.SplitHostPort(a)
	if err != nil {
		return fmt.Errorf("invalid address %q", a)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("address must use an IP literal: %q", a)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("invalid port in %q", a)
	}
	if ip.IsMulticast() || (!listen && ip.IsUnspecified()) {
		return fmt.Errorf("invalid IP in %q", a)
	}
	return nil
}

func validHost(host string) bool {
	if host == "*" {
		return true
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-", r) {
				return false
			}
		}
	}
	return true
}

func (c Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".velora-config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0640); err != nil {
		file.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync temporary config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
