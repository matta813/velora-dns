package dns

import (
	"context"
	"fmt"
	"strings"
	"time"

	wire "github.com/miekg/dns"
)

type UpstreamObserver interface {
	Upstream(server string, failed bool)
}
type Forwarder struct {
	Upstreams []string
	Timeout   time.Duration
	Retries   int
	Observer  UpstreamObserver
}

func (f *Forwarder) Resolve(ctx context.Context, q *wire.Msg) (*wire.Msg, string, error) {
	var last error
	for round := 0; round <= f.Retries; round++ {
		for _, upstream := range f.Upstreams {
			if err := ctx.Err(); err != nil {
				return nil, "", err
			}
			attempt, cancel := context.WithTimeout(ctx, f.Timeout)
			request := q.Copy()
			request.Id = wire.Id()
			request.AuthenticatedData = false
			if opt := request.IsEdns0(); opt != nil {
				opt.SetUDPSize(1232)
			}
			client := &wire.Client{Net: "udp", Timeout: f.Timeout, UDPSize: 1232}
			m, _, err := client.ExchangeContext(attempt, request, upstream)
			if err == nil && m.Truncated {
				client.Net = "tcp"
				m, _, err = client.ExchangeContext(attempt, request, upstream)
			}
			cancel()
			if err == nil && (!m.Response || !sameQuestion(q, m)) {
				err = fmt.Errorf("invalid upstream response")
			}
			if err == nil && (m.Rcode == wire.RcodeServerFailure || m.Rcode == wire.RcodeRefused) {
				err = fmt.Errorf("upstream response %s", wire.RcodeToString[m.Rcode])
			}
			if f.Observer != nil {
				f.Observer.Upstream(upstream, err != nil)
			}
			if err != nil {
				last = err
				continue
			}
			m.Id = q.Id
			m.Question = append([]wire.Question(nil), q.Question...)
			m.AuthenticatedData = false
			return m, upstream, nil
		}
	}
	return nil, "", fmt.Errorf("all upstream attempts failed: %v", last)
}
func sameQuestion(a, b *wire.Msg) bool {
	return len(a.Question) == 1 && len(b.Question) == 1 && strings.EqualFold(a.Question[0].Name, b.Question[0].Name) && a.Question[0].Qtype == b.Question[0].Qtype && a.Question[0].Qclass == b.Question[0].Qclass
}
