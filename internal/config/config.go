// Package config loads strict YAML configuration with explicit environment overrides.
package config

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type DNS struct {
	Listen         []string      `yaml:"listen" json:"listen"`
	Upstreams      []string      `yaml:"upstreams" json:"upstreams"`
	AllowedClients []string      `yaml:"allowed_clients" json:"allowed_clients"`
	Timeout        time.Duration `yaml:"timeout" json:"timeout"`
	Retries        int           `yaml:"retries" json:"retries"`
	MaxConcurrent  int           `yaml:"max_concurrent" json:"max_concurrent"`
}
type Cache struct {
	MaxEntries int `yaml:"max_entries" json:"max_entries"`
}
type HTTP struct {
	Listen string `yaml:"listen" json:"listen"`
	WebDir string `yaml:"web_dir" json:"web_dir"`
}
type Config struct {
	DNS          DNS    `yaml:"dns" json:"dns"`
	Cache        Cache  `yaml:"cache" json:"cache"`
	HTTP         HTTP   `yaml:"http" json:"http"`
	DatabasePath string `yaml:"database_path" json:"-"`
	LogLevel     string `yaml:"log_level" json:"log_level"`
}

func Default() Config {
	return Config{DNS: DNS{Listen: []string{"127.0.0.1:5353"}, Upstreams: []string{"1.1.1.1:53", "9.9.9.9:53"}, AllowedClients: []string{"127.0.0.0/8", "::1/128"}, Timeout: 2 * time.Second, Retries: 1, MaxConcurrent: 256}, Cache: Cache{MaxEntries: 10000}, HTTP: HTTP{Listen: "127.0.0.1:8080", WebDir: "web/dist"}, DatabasePath: "data/velora.db", LogLevel: "info"}
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
	for key, target := range map[string]*string{"HTTP_LISTEN": &c.HTTP.Listen, "WEB_DIR": &c.HTTP.WebDir, "DATABASE_PATH": &c.DatabasePath, "LOG_LEVEL": &c.LogLevel} {
		if v, ok := lookup("VELORA_" + key); ok {
			*target = v
		}
	}
	for key, target := range map[string]*[]string{"DNS_LISTEN": &c.DNS.Listen, "DNS_UPSTREAMS": &c.DNS.Upstreams, "DNS_ALLOWED_CLIENTS": &c.DNS.AllowedClients} {
		if v, ok := lookup("VELORA_" + key); ok {
			*target = strings.Split(v, ",")
			for i := range *target {
				(*target)[i] = strings.TrimSpace((*target)[i])
			}
		}
	}
	for key, target := range map[string]*int{"DNS_RETRIES": &c.DNS.Retries, "DNS_MAX_CONCURRENT": &c.DNS.MaxConcurrent, "CACHE_MAX_ENTRIES": &c.Cache.MaxEntries} {
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
	return c, c.Validate()
}
func (c Config) Validate() error {
	if len(c.DNS.Listen) == 0 || len(c.DNS.Listen) > 8 || len(c.DNS.Upstreams) == 0 || len(c.DNS.Upstreams) > 8 {
		return fmt.Errorf("DNS requires 1–8 listeners and upstreams")
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
	if c.Cache.MaxEntries < 0 || c.Cache.MaxEntries > 1000000 || c.DNS.MaxConcurrent < 1 || c.DNS.MaxConcurrent > 10000 {
		return fmt.Errorf("invalid cache or concurrency limit")
	}
	if c.DatabasePath == "" || c.HTTP.WebDir == "" {
		return fmt.Errorf("database_path and web_dir cannot be empty")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log_level")
	}
	return nil
}
func address(a string, listen bool) error {
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
