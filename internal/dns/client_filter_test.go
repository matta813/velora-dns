package dns

import (
	"context"
	"net/netip"
	"testing"

	"github.com/matta813/velora-dns/internal/cache"
	wire "github.com/miekg/dns"
)

type perClientFilter struct{ bypass netip.Addr }

func (perClientFilter) Blocked(name string) bool { return name == "ads.example." }
func (f perClientFilter) BlockedFor(client netip.Addr, name string) bool {
	return client != f.bypass && name == "ads.example."
}

type answerUpstream struct{}

func (answerUpstream) Resolve(_ context.Context, q *wire.Msg) (*wire.Msg, string, error) {
	m := new(wire.Msg)
	m.SetReply(q)
	rr, _ := wire.NewRR(q.Question[0].Name + " 60 IN A 192.0.2.1")
	m.Answer = append(m.Answer, rr)
	return m, "test", nil
}

func TestResolverUsesClientFilter(t *testing.T) {
	bypass := netip.MustParseAddr("192.0.2.50")
	resolver := NewResolver(nil, perClientFilter{bypass: bypass}, cache.New(10), answerUpstream{}, "NXDOMAIN")
	q := new(wire.Msg)
	q.SetQuestion("ads.example.", wire.TypeA)
	for _, tc := range []struct {
		ctx    context.Context
		source string
	}{
		{context.Background(), "blocked"},
		{WithClient(context.Background(), netip.MustParseAddr("192.0.2.9")), "blocked"},
		{WithClient(context.Background(), bypass), "upstream"},
		// The cached answer is re-checked for every client.
		{WithClient(context.Background(), netip.MustParseAddr("192.0.2.9")), "blocked"},
		{WithClient(context.Background(), bypass), "cache"},
	} {
		result, err := resolver.Resolve(tc.ctx, q.Copy())
		if err != nil || result.Source != tc.source {
			t.Fatalf("got %q %v, want %q", result.Source, err, tc.source)
		}
	}
	if _, ok := ClientFrom(WithClient(context.Background(), netip.Addr{})); ok {
		t.Fatal("invalid address should not count as a client")
	}
}
