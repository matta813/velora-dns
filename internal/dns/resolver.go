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
	return r.resolve(ctx, q, 0)
}
func (r *Resolver) resolve(ctx context.Context, q *wire.Msg, depth int) (Result, error) {
	if depth >= 16 {
		return Result{Source: "local"}, fmt.Errorf("CNAME resolution depth exceeded")
	}
	if err := ctx.Err(); err != nil {
		return Result{Source: "local"}, err
	}
	if len(q.Question) == 1 && r.blockedName(ctx, q.Question[0].Name) {
		return r.blocked(q), nil
	}
	if r.Local != nil {
		if m, ok := r.Local.Lookup(q); ok {
			if r.blockedAnswer(ctx, m) {
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
		if r.blockedAnswer(ctx, m) {
			return r.blocked(q), nil
		}
		return Result{Message: m, Source: "cache"}, nil
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
