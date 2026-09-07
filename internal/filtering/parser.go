package filtering

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"strings"
)

// ParseHosts accepts one domain per line or a hosts address followed by aliases.
// Malformed/oversized input never silently publishes a partial list.
func ParseHosts(data []byte) ([]string, error) {
	if len(data) > maxListBytes {
		return nil, fmt.Errorf("%w: source exceeds 8 MiB", ErrInvalid)
	}
	seen := map[string]bool{}
	out := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024), 64<<10)
	for line := 1; scanner.Scan(); line++ {
		text, _, _ := strings.Cut(scanner.Text(), "#")
		fields := strings.Fields(strings.TrimPrefix(text, "\ufeff"))
		if len(fields) == 0 {
			continue
		}
		if net.ParseIP(fields[0]) != nil {
			fields = fields[1:]
			if len(fields) == 0 {
				return nil, fmt.Errorf("%w: missing hosts alias on line %d", ErrInvalid, line)
			}
		} else if len(fields) != 1 {
			return nil, fmt.Errorf("%w: invalid domain line %d", ErrInvalid, line)
		}
		for _, field := range fields {
			wildcard := strings.HasPrefix(field, "*.")
			domain, err := name(strings.TrimPrefix(field, "*."))
			if err != nil || net.ParseIP(domain) != nil {
				return nil, fmt.Errorf("%w: invalid domain on line %d", ErrInvalid, line)
			}
			if domain == "localhost" || domain == "localhost.localdomain" || domain == "ip6-localhost" || domain == "ip6-loopback" || domain == "broadcasthost" {
				continue
			}
			if wildcard {
				domain = "*." + domain
			}
			if !seen[domain] {
				seen[domain] = true
				out = append(out, domain)
				if len(out) > MaxDomains {
					return nil, fmt.Errorf("%w: too many domains", ErrInvalid)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: oversized or unreadable line", ErrInvalid)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: list has no valid domains", ErrInvalid)
	}
	return out, nil
}
