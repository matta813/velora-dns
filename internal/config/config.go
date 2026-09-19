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
	MaxEntries int `yaml:"max_entries" json:"max_entries"`
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

type Cluster struct {
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
	Cluster        Cluster    `yaml:"cluster" json:"cluster"`
	Node           Node       `yaml:"node" json:"node"`
	Management     Management `yaml:"-" json:"-"`
	DatabasePath   string     `yaml:"database_path" json:"-"`
	DatabaseDriver string     `yaml:"database_driver" json:"-"`
	DatabaseURL    string     `yaml:"database_url" json:"-"`
	LogLevel       string     `yaml:"log_level" json:"log_level"`
}

func Default() Config {
	return Config{DNS: DNS{Listen: []string{"127.0.0.1:3535"}, Upstreams: []string{"1.1.1.1:53", "9.9.9.9:53"}, AllowedClients: []string{"127.0.0.0/8", "::1/128"}, Timeout: 2 * time.Second, Retries: 1, MaxConcurrent: 256, GlobalQPS: 1000, ClientQPS: 100, RateLimitBurst: 100, RateLimitEnabled: false, MaxTCPConns: 256}, Cache: Cache{MaxEntries: 10000}, HTTP: HTTP{Listen: "127.0.0.1:8080", WebDir: "web/dist", AllowedHosts: []string{"localhost", "127.0.0.1", "::1"}}, QueryLog: QueryLog{MaxRows: 100000, QueueSize: 1024, Retention: 7 * 24 * time.Hour}, Cluster: Cluster{MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: 5 * time.Minute}, Node: Node{ID: "node-1", Name: "Primary"}, DatabasePath: "data/velora.db", DatabaseDriver: "sqlite", LogLevel: "info"}
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
	for key, target := range map[string]*string{"HTTP_LISTEN": &c.HTTP.Listen, "WEB_DIR": &c.HTTP.WebDir, "DATABASE_PATH": &c.DatabasePath, "DATABASE_DRIVER": &c.DatabaseDriver, "DATABASE_URL": &c.DatabaseURL, "LOG_LEVEL": &c.LogLevel, "FILTERING_BLOCK_MODE": &c.Filtering.BlockMode, "DNS_DOT_LISTEN": &c.DNS.DoTListen, "DNS_DOH_LISTEN": &c.DNS.DoHListen, "DNS_DOQ_LISTEN": &c.DNS.DoQListen, "DNS_TLS_CERT_FILE": &c.DNS.TLSCertFile, "DNS_TLS_KEY_FILE": &c.DNS.TLSKeyFile} {
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
			*target = strings.Split(v, ",")
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
func (c Config) Validate() error {
	if (c.Management.BootstrapUsername == "") != (c.Management.BootstrapPassword == "") || (c.Management.BootstrapPassword != "" && len(c.Management.BootstrapPassword) < 12) {
		return fmt.Errorf("bootstrap username and password must both be set; password requires at least 12 characters")
	}
	if c.Filtering.BlockMode != "" && c.Filtering.BlockMode != "NXDOMAIN" && c.Filtering.BlockMode != "ZERO" {
		return fmt.Errorf("filtering.block_mode must be NXDOMAIN or ZERO")
	}
	if len(c.DNS.Listen) == 0 || len(c.DNS.Listen) > 8 || len(c.DNS.Upstreams) == 0 || len(c.DNS.Upstreams) > 8 {
		return fmt.Errorf("DNS requires 1–8 listeners and upstreams")
	}
	if (c.DNS.TLSCertFile == "") != (c.DNS.TLSKeyFile == "") {
		return fmt.Errorf("dns.tls_cert_file and dns.tls_key_file must both be set")
	}
	if c.DNS.DNSSEC && len(c.DNS.TrustAnchors) == 0 {
		return fmt.Errorf("dns.trust_anchors is required when DNSSEC validation is enabled")
	}
	for _, anchor := range c.DNS.TrustAnchors {
		rr, err := wire.NewRR(anchor)
		if err != nil || rr.Header().Rrtype != wire.TypeDS {
			return fmt.Errorf("invalid DNSSEC DS trust anchor %q", anchor)
		}
	}
	for name, value := range map[string]string{"dot": c.DNS.DoTListen, "doh": c.DNS.DoHListen, "doq": c.DNS.DoQListen} {
		if value != "" {
			if c.DNS.TLSCertFile == "" {
				return fmt.Errorf("DNS-over-%s requires TLS certificate and key", name)
			}
			if err := address(value, true); err != nil {
				return fmt.Errorf("invalid DNS-over-%s listener: %w", name, err)
			}
		}
	}
	seen := map[string]bool{}
	for _, a := range c.DNS.Listen {
		if err := address(a, true); err != nil {
			return err
		}
		if seen[a] {
			return fmt.Errorf("duplicate listener")
		}
		seen[a] = true
	}
	for _, a := range c.DNS.Upstreams {
		if err := address(a, false); err != nil {
			return err
		}
		if seen[a] {
			return fmt.Errorf("upstream equals listener")
		}
	}
	if err := address(c.HTTP.Listen, true); err != nil {
		return err
	}
	if len(c.HTTP.AllowedHosts) == 0 {
		return fmt.Errorf("http.allowed_hosts cannot be empty")
	}
	for _, host := range c.HTTP.AllowedHosts {
		if !validHost(host) {
			return fmt.Errorf("invalid HTTP allowed host %q", host)
		}
	}
	if len(c.DNS.AllowedClients) == 0 {
		return fmt.Errorf("allowed_clients cannot be empty")
	}
	for _, v := range c.DNS.AllowedClients {
		if _, err := netip.ParsePrefix(v); err != nil {
			return fmt.Errorf("invalid client CIDR %q", v)
		}
	}
	if c.DNS.Timeout < 10*time.Millisecond || c.DNS.Timeout > 10*time.Second || c.DNS.Retries < 0 || c.DNS.Retries > 3 {
		return fmt.Errorf("timeout must be 10ms–10s and retries 0–3")
	}
	if c.Cache.MaxEntries < 0 || c.Cache.MaxEntries > 1000000 || c.DNS.MaxConcurrent < 1 || c.DNS.MaxConcurrent > 10000 || c.DNS.GlobalQPS < 1 || c.DNS.GlobalQPS > 100000 || c.DNS.ClientQPS < 1 || c.DNS.ClientQPS > 100000 || c.DNS.RateLimitBurst < 1 || c.DNS.RateLimitBurst > 100000 || c.DNS.MaxTCPConns < 1 || c.DNS.MaxTCPConns > 100000 {
		return fmt.Errorf("invalid cache or concurrency limit")
	}
	if c.QueryLog.QueueSize < 1 || c.QueryLog.QueueSize > 100000 || c.QueryLog.Retention < time.Minute || c.QueryLog.Retention > 365*24*time.Hour || c.QueryLog.MaxRows < 1 || c.QueryLog.MaxRows > 1000000 {
		return fmt.Errorf("invalid query_log settings")
	}
	if c.DatabaseDriver == "" {
		c.DatabaseDriver = "sqlite"
	}
	if c.DatabaseDriver != "sqlite" && c.DatabaseDriver != "postgres" {
		return fmt.Errorf("database_driver must be sqlite or postgres")
	}
	if c.DatabaseDriver == "postgres" && c.DatabaseURL == "" {
		return fmt.Errorf("database_url is required when database_driver is postgres")
	}
	if c.DatabaseDriver == "sqlite" && c.DatabasePath == "" {
		return fmt.Errorf("database_path cannot be empty for sqlite")
	}
	if c.HTTP.WebDir == "" {
		return fmt.Errorf("web_dir cannot be empty")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log_level")
	}
	return nil
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
