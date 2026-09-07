package filtering

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const maxListBytes = 8 << 20

// FetchHosts downloads a public hostname list. Private, loopback and link-local
// targets are rejected before the request to keep management endpoints out of SSRF paths.
func FetchHosts(ctx context.Context, rawURL string, client *http.Client, resolve func(context.Context, string) ([]net.IPAddr, error)) ([]string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid source URL")
	}
	if u.Port() != "" && u.Port() != "80" && u.Port() != "443" {
		return nil, fmt.Errorf("source URL uses disallowed port")
	}
	ips, err := resolve(ctx, u.Hostname())
	if err != nil {
		return nil, fmt.Errorf("resolve source host: %w", err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("source host has no addresses")
	}
	for _, ip := range ips {
		if blockedAddress(ip.IP) {
			return nil, fmt.Errorf("source host resolves to a private address")
		}
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download source: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("source returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxListBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxListBytes {
		return nil, fmt.Errorf("source exceeds 8 MiB")
	}
	return ParseHosts(data), nil
}
func ParseHosts(data []byte) []string {
	seen := map[string]struct{}{}
	out := []string{}
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Buffer(make([]byte, 1024), 64<<10)
	for s.Scan() {
		line := strings.TrimSpace(strings.SplitN(s.Text(), "#", 2)[0])
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		candidate := fields[0]
		if net.ParseIP(candidate) != nil && len(fields) > 1 {
			candidate = fields[1]
		}
		if domain, err := name(candidate); err == nil {
			if _, ok := seen[domain]; !ok {
				seen[domain] = struct{}{}
				out = append(out, domain)
			}
		}
	}
	return out
}
func blockedAddress(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	return !ok || !a.IsGlobalUnicast() || a.IsPrivate()
}
