package dns

import (
	"context"

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
type Resolver struct {
	Cache     *cache.Cache
	Forwarder Upstream
}

func (r *Resolver) Resolve(ctx context.Context, q *wire.Msg) (Result, error) {
	if m, ok := r.Cache.Get(q); ok {
		return Result{Message: m, Source: "cache"}, nil
	}
	m, upstream, err := r.Forwarder.Resolve(ctx, q)
	if err != nil {
		return Result{Source: "upstream"}, err
	}
	r.Cache.Put(q, m)
	return Result{Message: m, Source: "upstream", Upstream: upstream}, nil
}
