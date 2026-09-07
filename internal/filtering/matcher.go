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
		n, err := name(r.Domain)
		if err != nil {
			return nil, err
		}
		r.Domain = n
		if old, ok := m.rules[key(r)]; ok && old.Action != r.Action {
			return nil, fmt.Errorf("conflicting rule for %s", r.Domain)
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
	if r, ok := m.rules["="+n]; ok {
		return r.Action, true
	}
	for current := n; ; {
		if r, ok := m.rules["*"+current]; ok {
			return r.Action, true
		}
		i := strings.IndexByte(current, '.')
		if i < 0 {
			break
		}
		current = current[i+1:]
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
	v = strings.TrimPrefix(v, "*.")
	v = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
	if v == "" || len(v) > 253 {
		return "", fmt.Errorf("invalid domain")
	}
	for _, label := range strings.Split(v, ".") {
		if len(label) == 0 || len(label) > 63 {
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
