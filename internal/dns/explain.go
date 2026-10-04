package dns

import (
	"context"
	"net/netip"
	"strings"

	wire "github.com/miekg/dns"
)

// RuleRef identifies the rule, zone, policy or list behind a decision.
type RuleRef struct {
	ID     int64  `json:"id,omitempty"`
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
}

// LocalMatch is a local answer plus the layer ("rewrite" or "zone") and rules
// that produced it.
type LocalMatch struct {
	Msg   *wire.Msg
	Stage string
	Rules []RuleRef
}

// LocalExplainer is implemented by a Local that can say what answered.
type LocalExplainer interface {
	ExplainLocal(*wire.Msg) (LocalMatch, bool)
}

// RouteExplainer is implemented by an Upstream that picks a rule per name.
type RouteExplainer interface {
	Route(name string) (RuleRef, bool)
}

// FilterInfo labels a filter decision. It never decides anything.
type FilterInfo struct {
	Scope   string    `json:"scope"` // global or client_policy
	Client  string    `json:"client,omitempty"`
	Mode    string    `json:"mode,omitempty"`
	Matches []RuleRef `json:"matches,omitempty"`
	Allowed []RuleRef `json:"allowed_by,omitempty"`
}

// FilterExplainer is implemented by a Filter that can attribute decisions.
type FilterExplainer interface {
	ExplainFilter(client netip.Addr, known bool, name string) FilterInfo
}

// Step is one stage the resolver considered, in order.
type Step struct {
	Stage string `json:"stage"` // filter, rewrite, zone, cache, forwarding, upstream
	// Result is passed, blocked, allowed, no_match, answered, miss, hit,
	// refused or would_forward.
	Result string      `json:"result"`
	Rules  []RuleRef   `json:"rules,omitempty"`
	Filter *FilterInfo `json:"filter,omitempty"`
	TTL    *uint32     `json:"remaining_ttl,omitempty"`
	// Depth is greater than zero for steps taken for a CNAME target.
	Depth  int    `json:"depth"`
	Detail string `json:"detail,omitempty"`
}

// Explanation is the result of a dry run.
type Explanation struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Client  string   `json:"client,omitempty"`
	Source  string   `json:"source"` // what Resolve would report: local, cache, blocked, upstream, refused
	Winner  Step     `json:"winner"`
	Rcode   string   `json:"rcode,omitempty"`
	Answers []string `json:"answers"`
	Steps   []Step   `json:"steps"`
}

// trace records steps of a dry run. A nil trace means a real query, and all
// methods are then no-ops, so resolve has a single code path for both.
type trace struct{ steps []Step }

func (t *trace) add(depth int, s Step) {
	if t != nil {
		s.Depth = depth
		t.steps = append(t.steps, s)
	}
}

// ParseType maps a record type name to a type the resolver serves.
func ParseType(name string) (uint16, bool) {
	t, ok := wire.StringToType[strings.ToUpper(name)]
	return t, ok && supported(t)
}

// Explain evaluates the resolver's decision for one query without side
// effects: it reads the cache with Peek, never contacts an upstream and
// touches no counters or logs. client may be the zero Addr. The query always
// asks for recursion, like stub resolvers do.
func (r *Resolver) Explain(ctx context.Context, name string, qtype uint16, client netip.Addr) Explanation {
	q := new(wire.Msg)
	q.SetQuestion(wire.Fqdn(name), qtype)
	q.RecursionDesired = true
	if client.IsValid() {
		ctx = WithClient(ctx, client.Unmap())
	}
	t := &trace{}
	result, err := r.resolve(ctx, q, 0, t)
	out := Explanation{Name: q.Question[0].Name, Type: wire.TypeToString[qtype], Source: result.Source, Steps: t.steps, Answers: []string{}}
	if client.IsValid() {
		out.Client = client.Unmap().String()
	}
	if err != nil {
		out.Winner = Step{Stage: result.Source, Result: "error", Detail: err.Error()}
		return out
	}
	if result.Message != nil {
		out.Rcode = wire.RcodeToString[result.Message.Rcode]
		for _, rr := range result.Message.Answer {
			out.Answers = append(out.Answers, rr.String())
		}
	}
	// The winner is the first terminal step, or the step that blocked a
	// CNAME target or an answer.
	for _, s := range t.steps {
		switch s.Result {
		case "blocked", "answered", "hit", "refused", "would_forward":
			if out.Winner.Stage == "" || (result.Source == "blocked" && s.Result == "blocked") {
				out.Winner = s
			}
		}
	}
	return out
}
