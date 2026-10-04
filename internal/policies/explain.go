package policies

import (
	"net/netip"

	"github.com/matta813/velora-dns/internal/dns"
	"github.com/matta813/velora-dns/internal/filtering"
)

// attributor is implemented by the filtering service.
type attributor interface {
	Attribute(name string, sourceIDs []int64) filtering.Attribution
}

// ExplainFilter labels the filter decision for name (see dns.FilterExplainer).
// It reads the same snapshots as Blocked/BlockedFor but only describes them;
// the verdict itself always comes from those methods.
func (s *Service) ExplainFilter(addr netip.Addr, known bool, name string) dns.FilterInfo {
	info := dns.FilterInfo{Scope: "global"}
	var ids []int64 // nil: every enabled source
	var extra *filtering.Matcher
	var policy *Policy
	if known {
		eff := s.Effective(addr)
		if eff.Policy != nil && eff.Mode != ModeDefault {
			policy = eff.Policy
			info = dns.FilterInfo{Scope: "client_policy", Client: eff.Client.Name, Mode: policy.Mode}
		}
	}
	if policy != nil {
		if policy.Mode == ModeDisabled {
			return info
		}
		ids = append([]int64{}, policy.Blocklists...)
		rules := make([]filtering.Rule, 0, len(policy.Allow)+len(policy.Block))
		for _, d := range policy.Block {
			rules = append(rules, filtering.Rule{Domain: d, Wildcard: true, Action: filtering.Block})
		}
		for _, d := range policy.Allow {
			rules = append(rules, filtering.Rule{Domain: d, Wildcard: true, Action: filtering.Allow})
		}
		extra, _ = filtering.New(rules)
	}
	attr, ok := s.global.(attributor)
	if !ok {
		return info
	}
	a := attr.Attribute(name, ids)
	add := func(action filtering.Action, ref dns.RuleRef) {
		if action == filtering.Allow {
			info.Allowed = append(info.Allowed, ref)
		} else {
			info.Matches = append(info.Matches, ref)
		}
	}
	if a.StaticMatched {
		add(a.Static, dns.RuleRef{Name: "filtering.blocklist configuration rule"})
	}
	for _, source := range a.Sources {
		info.Matches = append(info.Matches, dns.RuleRef{ID: source.ID, Name: source.Name, Detail: "blocklist"})
	}
	if extra != nil {
		if action, matched := extra.Match(name); matched {
			add(action, dns.RuleRef{ID: policy.ID, Name: "client policy domains"})
		}
	}
	return info
}
