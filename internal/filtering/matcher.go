// Package filtering implements immutable domain policy snapshots.
package filtering

import (
	"fmt"
	"strings"
)

type Action uint8

const (
	Allow Action = iota
	Block
)

type Rule struct {
	Domain   string
	Wildcard bool
	Action   Action
}
type Matcher struct{ rules map[string]Rule }

// New validates rules once, so DNS lookups are allocation-free apart from normalization.
func New(rules []Rule) (*Matcher, error) {
	m := &Matcher{rules: make(map[string]Rule, len(rules))}
	for _, r := range rules {
		if strings.HasPrefix(strings.TrimSpace(r.Domain), "*.") {
			r.Wildcard = true
		}
		n, err := name(r.Domain)
		if err != nil {
			return nil, err
		}
		r.Domain = n
		if old, ok := m.rules[key(r)]; ok && old.Action == Allow {
			continue
		}
		m.rules[key(r)] = r
	}
	return m, nil
}
func (m *Matcher) Match(domain string) (Action, bool) {
	n, err := name(domain)
	if err != nil {
		return Allow, false
	}
	// An exact allowlist entry always wins; wildcard matching is boundary-aware.
	blocked := false
	if r, ok := m.rules["="+n]; ok {
		if r.Action == Allow {
			return Allow, true
		}
		blocked = true
	}
	for current := n; ; {
		if r, ok := m.rules["*"+current]; ok {
			if r.Action == Allow {
				return Allow, true
			}
			blocked = true
		}
		i := strings.IndexByte(current, '.')
		if i < 0 {
			break
		}
		current = current[i+1:]
	}
	if blocked {
		return Block, true
	}
	return Allow, false
}
func (m *Matcher) Blocked(domain string) bool {
	action, ok := m.Match(domain)
	return ok && action == Block
}
func key(r Rule) string {
	if r.Wildcard {
		return "*" + r.Domain
	}
	return "=" + r.Domain
}
func name(v string) (string, error) {
	v = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
	v = strings.TrimPrefix(v, "*.")
	if v == "" || len(v) > 253 {
		return "", fmt.Errorf("invalid domain")
	}
	for _, label := range strings.Split(v, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid domain")
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return "", fmt.Errorf("invalid domain")
			}
		}
	}
	return v, nil
}
