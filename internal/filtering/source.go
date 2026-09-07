package filtering

import (
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

type resolveHost func(context.Context, string) ([]net.IPAddr, error)
type dialHost func(context.Context, string, string) (net.Conn, error)

func ValidateSourceURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.User != nil || u.Hostname() == "" || u.Opaque != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%w: use an HTTP(S) URL without credentials or fragments", ErrInvalid)
	}
	if u.Port() != "" && u.Port() != "80" && u.Port() != "443" {
		return fmt.Errorf("%w: only ports 80 and 443 are allowed", ErrInvalid)
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil {
		if !publicAddress(ip) {
			return fmt.Errorf("%w: source address must be public", ErrInvalid)
		}
	} else if _, err := name(u.Hostname()); err != nil {
		return fmt.Errorf("%w: invalid source hostname", ErrInvalid)
	}
	return nil
}
func FetchHosts(ctx context.Context, raw string) ([]string, error) {
	return fetchHosts(ctx, raw, net.DefaultResolver.LookupIPAddr, (&net.Dialer{Timeout: 3 * time.Second}).DialContext)
}

// The transport resolves and validates inside DialContext, then connects to that
// exact IP. No environment proxy or second hostname lookup can bypass validation.
func fetchHosts(parent context.Context, raw string, resolve resolveHost, dial dialHost) ([]string, error) {
	if err := ValidateSourceURL(raw); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 16 << 10, TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 3 * time.Second}
	defer transport.CloseIdleConnections()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := resolve(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("source resolution failed")
		}
		if len(ips) == 0 || len(ips) > 16 {
			return nil, fmt.Errorf("invalid source address count")
		}
		for _, ip := range ips {
			addr, ok := netip.AddrFromSlice(ip.IP)
			if !ok || !publicAddress(addr) {
				return nil, fmt.Errorf("source resolves to a non-public address")
			}
		}
		var last error
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("source download failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("source returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxListBytes {
		return nil, fmt.Errorf("source exceeds 8 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxListBytes+1))
	if err != nil {
		return nil, fmt.Errorf("source read failed")
	}
	if len(data) > maxListBytes {
		return nil, fmt.Errorf("source exceeds 8 MiB")
	}
	return ParseHosts(data)
}
func publicAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	// Special-use ranges may still satisfy IsGlobalUnicast.
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16", "64:ff9b::/96", "64:ff9b:1::/48"} {
		if netip.MustParsePrefix(prefix).Contains(addr) {
			return false
		}
	}
	return addr.Is4() || strings.HasPrefix(addr.String(), "2") || strings.HasPrefix(addr.String(), "3")
}
