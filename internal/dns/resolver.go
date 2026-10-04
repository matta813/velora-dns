package dns

import (
	"context"
	"fmt"
	"net/netip"
	"sync/atomic"

	"github.com/matta813/velora-dns/internal/cache"
	wire "github.com/miekg/dns"
)

type Upstream interface {
	Resolve(context.Context, *wire.Msg) (*wire.Msg, string, error)
}
type Result struct {
	Message          *wire.Msg
	Source, Upstream string
}
type Local interface {
	Lookup(*wire.Msg) (*wire.Msg, bool)
}
type Filter interface{ Blocked(string) bool }

// ClientFilter is a Filter whose decision can depend on the querying client.
type ClientFilter interface {
	Filter
	BlockedFor(client netip.Addr, name string) bool
}

type clientKey struct{}

// WithClient records the querying client's address for client-aware filters.
func WithClient(ctx context.Context, client netip.Addr) context.Context {
	return context.WithValue(ctx, clientKey{}, client)
}

// ClientFrom returns the address recorded by WithClient.
func ClientFrom(ctx context.Context) (netip.Addr, bool) {
	client, ok := ctx.Value(clientKey{}).(netip.Addr)
	return client, ok && client.IsValid()
}

// blockedName applies the client's policy when both a client-aware filter
// and a client address are available, and the global policy otherwise.
func (r *Resolver) blockedName(ctx context.Context, name string) bool {
	if r.Filter == nil {
		return false
	}
	if filter, ok := r.Filter.(ClientFilter); ok {
		if client, known := ClientFrom(ctx); known {
			return filter.BlockedFor(client, name)
		}
	}
	return r.Filter.Blocked(name)
}

type Resolver struct {
	blockMode atomic.Value // stores string
	Local     Local
	Filter    Filter
	Cache     *cache.Cache
	Forwarder Upstream
}

func NewResolver(local Local, filter Filter, cache *cache.Cache, forwarder Upstream, blockMode string) *Resolver {
	r := &Resolver{Local: local, Filter: filter, Cache: cache, Forwarder: forwarder}
	r.blockMode.Store(blockMode)
	return r
}

func (r *Resolver) SetBlockMode(mode string) { r.blockMode.Store(mode) }
func (r *Resolver) getBlockMode() string {
	if v := r.blockMode.Load(); v != nil {
		return v.(string)
	}
	return ""
}

func (r *Resolver) Resolve(ctx context.Context, q *wire.Msg) (Result, error) {
	return r.resolve(ctx, q, 0, nil)
}

// filterStep records a filter verdict for a dry run.
func (r *Resolver) filterStep(ctx context.Context, t *trace, depth int, name string, blocked bool, detail string) {
	if t == nil {
		return
	}
	step := Step{Stage: "filter", Result: "passed", Detail: detail}
	if blocked {
		step.Result = "blocked"
	}
	if explainer, ok := r.Filter.(FilterExplainer); ok {
		client, known := ClientFrom(ctx)
		info := explainer.ExplainFilter(client, known, name)
		step.Filter = &info
		if !blocked && len(info.Allowed) > 0 {
			step.Result = "allowed"
		}
	}
	t.add(depth, step)
}

// resolve answers q. A non-nil t makes it a dry run: the cache is only
// peeked, no upstream is contacted and nothing is stored, so decisions are
// explained by the exact code that makes them.
func (r *Resolver) resolve(ctx context.Context, q *wire.Msg, depth int, t *trace) (Result, error) {
	if depth >= 16 {
		return Result{Source: "local"}, fmt.Errorf("CNAME resolution depth exceeded")
	}
	if err := ctx.Err(); err != nil {
		return Result{Source: "local"}, err
	}
	if len(q.Question) == 1 {
		blocked := r.blockedName(ctx, q.Question[0].Name)
		if r.Filter != nil {
			r.filterStep(ctx, t, depth, q.Question[0].Name, blocked, "")
		}
		if blocked {
			return r.blocked(q), nil
		}
	}
	if r.Local != nil {
		var m *wire.Msg
		var ok bool
		if explainer, can := r.Local.(LocalExplainer); can && t != nil {
			var match LocalMatch
			if match, ok = explainer.ExplainLocal(q); ok {
				m = match.Msg
				t.add(depth, Step{Stage: match.Stage, Result: "answered", Rules: match.Rules})
			} else {
				t.add(depth, Step{Stage: "rewrite", Result: "no_match"})
				t.add(depth, Step{Stage: "zone", Result: "no_match"})
			}
		} else {
			m, ok = r.Local.Lookup(q)
		}
		if ok {
			if name, blocked := r.blockedAnswerName(ctx, m); blocked {
				r.filterStep(ctx, t, depth, name, true, "answer or CNAME target is blocked")
				return r.blocked(q), nil
			}
			if q.RecursionDesired && q.Question[0].Qtype != wire.TypeCNAME && m.Rcode == wire.RcodeSuccess && len(m.Ns) == 0 && len(m.Answer) > 0 {
				if cname, ok := m.Answer[len(m.Answer)-1].(*wire.CNAME); ok {
					target := q.Copy()
					target.Question[0].Name = cname.Target
					tail, err := r.resolve(ctx, target, depth+1, t)
					if err != nil {
						return Result{Source: "local"}, err
					}
					if tail.Source == "blocked" {
						return r.blocked(q), nil
					}
					m.Answer = append(m.Answer, tail.Message.Answer...)
					m.Ns = tail.Message.Ns
					m.Extra = tail.Message.Extra
					m.Rcode = tail.Message.Rcode
					return Result{Message: m, Source: "local", Upstream: tail.Upstream}, nil
				}
			}
			return Result{Message: m, Source: "local"}, nil
		}
	}
	if !q.RecursionDesired {
		m := new(wire.Msg)
		m.SetRcode(q, wire.RcodeRefused)
		m.RecursionAvailable = true
		t.add(depth, Step{Stage: "refused", Result: "refused", Detail: "recursion not desired"})
		return Result{Message: m, Source: "refused"}, nil
	}
	var m *wire.Msg
	var hit bool
	if t != nil {
		var ttl uint32
		if m, ttl, hit = r.Cache.Peek(q); hit {
			t.add(depth, Step{Stage: "cache", Result: "hit", TTL: &ttl})
		} else {
			t.add(depth, Step{Stage: "cache", Result: "miss"})
		}
	} else {
		m, hit = r.Cache.Get(q)
	}
	if hit {
		if name, blocked := r.blockedAnswerName(ctx, m); blocked {
			r.filterStep(ctx, t, depth, name, true, "cached answer or CNAME target is blocked")
			return r.blocked(q), nil
		}
		return Result{Message: m, Source: "cache"}, nil
	}
	if t != nil {
		step := Step{Stage: "upstream", Result: "would_forward", Detail: "global upstream resolvers; none were contacted"}
		if router, ok := r.Forwarder.(RouteExplainer); ok {
			if rule, found := router.Route(q.Question[0].Name); found {
				step = Step{Stage: "forwarding", Result: "would_forward", Rules: []RuleRef{rule}, Detail: "conditional forwarding rule; no upstream was contacted"}
			}
		}
		t.add(depth, step)
		empty := new(wire.Msg)
		empty.SetReply(q)
		return Result{Message: empty, Source: "upstream"}, nil
	}
	m, upstream, err := r.Forwarder.Resolve(ctx, q)
	if err != nil {
		return Result{Source: "upstream"}, err
	}
	if r.blockedAnswer(ctx, m) {
		return r.blocked(q), nil
	}
	cache.NormalizeNegativeTTL(m)
	r.Cache.NormalizeUpstreamTTL(m)
	r.Cache.Put(q, m)
	return Result{Message: m, Source: "upstream", Upstream: upstream}, nil
}
