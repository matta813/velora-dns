package dns

import (
	"context"
	"fmt"

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
type Resolver struct {
	BlockMode string
	Local     Local
	Filter    Filter
	Cache     *cache.Cache
	Forwarder Upstream
}

func (r *Resolver) Resolve(ctx context.Context, q *wire.Msg) (Result, error) {
	return r.resolve(ctx, q, 0)
}
func (r *Resolver) resolve(ctx context.Context, q *wire.Msg, depth int) (Result, error) {
	if depth >= 16 {
		return Result{Source: "local"}, fmt.Errorf("CNAME resolution depth exceeded")
	}
	if err := ctx.Err(); err != nil {
		return Result{Source: "local"}, err
	}
	if r.Filter != nil && len(q.Question) == 1 {
		if r.Filter.Blocked(q.Question[0].Name) {
			return r.blocked(q), nil
		}
	}
	if r.Local != nil {
		if m, ok := r.Local.Lookup(q); ok {
			if r.blockedAnswer(m) {
				return r.blocked(q), nil
			}
			if q.RecursionDesired && q.Question[0].Qtype != wire.TypeCNAME && m.Rcode == wire.RcodeSuccess && len(m.Ns) == 0 && len(m.Answer) > 0 {
				if cname, ok := m.Answer[len(m.Answer)-1].(*wire.CNAME); ok {
					target := q.Copy()
					target.Question[0].Name = cname.Target
					tail, err := r.resolve(ctx, target, depth+1)
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
		return Result{Message: m, Source: "refused"}, nil
	}
	if m, ok := r.Cache.Get(q); ok {
		if r.blockedAnswer(m) {
			return r.blocked(q), nil
		}
		return Result{Message: m, Source: "cache"}, nil
	}
	m, upstream, err := r.Forwarder.Resolve(ctx, q)
	if err != nil {
		return Result{Source: "upstream"}, err
	}
	if r.blockedAnswer(m) {
		return r.blocked(q), nil
	}
	cache.NormalizeNegativeTTL(m)
	r.Cache.Put(q, m)
	return Result{Message: m, Source: "upstream", Upstream: upstream}, nil
}
