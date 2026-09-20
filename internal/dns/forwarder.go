package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
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
	TLSConfig *tls.Config
	Validator *DNSSECValidator
	dohOnce   sync.Once
	dohClient *http.Client
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
			request.Extra = withoutOPT(request.Extra)
			if opt := q.IsEdns0(); opt != nil || f.Validator != nil {
				do := f.Validator != nil
				if opt != nil {
					do = do || opt.Do()
				}
				request.SetEdns0(1232, do)
			}
			address := strings.TrimPrefix(upstream, "tls://")
			var m *wire.Msg
			var err error
			if strings.HasPrefix(upstream, "https://") {
				m, err = f.exchangeDoH(attempt, request, upstream)
			} else {
				client := &wire.Client{Net: "udp", Timeout: f.Timeout, UDPSize: 1232}
				if address != upstream {
					host, _, splitErr := net.SplitHostPort(address)
					if splitErr != nil {
						cancel()
						return nil, "", splitErr
					}
					client.Net = "tcp-tls"
					client.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
					if f.TLSConfig != nil {
						client.TLSConfig = f.TLSConfig.Clone()
						if client.TLSConfig.ServerName == "" {
							client.TLSConfig.ServerName = host
						}
					}
				}
				m, _, err = client.ExchangeContext(attempt, request, address)
				if err == nil && m.Truncated {
					if address == upstream {
						client.Net = "tcp"
					}
					m, _, err = client.ExchangeContext(attempt, request, address)
				}
			}
			cancel()
			if err == nil && (!m.Response || !sameQuestion(q, m)) {
				err = fmt.Errorf("invalid upstream response")
			}
			if err == nil {
				err = validateAnswerChain(q, m)
			}
			if err == nil && (m.Rcode == wire.RcodeServerFailure || m.Rcode == wire.RcodeRefused) {
				err = fmt.Errorf("upstream response %s", wire.RcodeToString[m.Rcode])
			}
			if err == nil && f.Validator != nil && !q.CheckingDisabled {
				status, validationErr := f.Validator.Validate(ctx, q, m, func(validationCtx context.Context, validationQuery *wire.Msg) (*wire.Msg, error) {
					validatorForwarder := &Forwarder{Upstreams: []string{upstream}, Timeout: f.Timeout, TLSConfig: f.TLSConfig}
					validationResponse, _, exchangeErr := validatorForwarder.Resolve(validationCtx, validationQuery)
					return validationResponse, exchangeErr
				})
				if validationErr != nil {
					err = fmt.Errorf("DNSSEC bogus: %w", validationErr)
				} else {
					m.AuthenticatedData = status == DNSSECSecure
				}
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
			if f.Validator == nil || q.CheckingDisabled {
				m.AuthenticatedData = false
			}
			if q.IsEdns0() == nil {
				m.Extra = withoutOPT(m.Extra)
			} else if opt := m.IsEdns0(); opt != nil {
				opt.Option = nil
			}
			if opt := q.IsEdns0(); opt == nil || !opt.Do() {
				m.Answer = withoutDNSSEC(m.Answer)
				m.Ns = withoutDNSSEC(m.Ns)
				m.Extra = withoutDNSSEC(m.Extra)
			}
			return m, upstream, nil
		}
	}
	return nil, "", fmt.Errorf("all upstream attempts failed: %v", last)
}

func withoutDNSSEC(records []wire.RR) []wire.RR {
	out := make([]wire.RR, 0, len(records))
	for _, rr := range records {
		switch rr.Header().Rrtype {
		case wire.TypeRRSIG, wire.TypeNSEC, wire.TypeNSEC3, wire.TypeDNSKEY, wire.TypeDS:
			continue
		default:
			out = append(out, rr)
		}
	}
	return out
}

func (f *Forwarder) exchangeDoH(ctx context.Context, query *wire.Msg, upstream string) (*wire.Msg, error) {
	payload, err := query.Pack()
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", dnsMessageMediaType)
	request.Header.Set("Accept", dnsMessageMediaType)
	client := f.httpClient()
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || mediaErr != nil || mediaType != dnsMessageMediaType {
		return nil, fmt.Errorf("invalid DoH response")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 65536))
	if err != nil || len(body) > 65535 {
		return nil, fmt.Errorf("invalid DoH response size")
	}
	message := new(wire.Msg)
	if err = message.Unpack(body); err != nil {
		return nil, err
	}
	return message, nil
}

func (f *Forwarder) httpClient() *http.Client {
	f.dohOnce.Do(func() {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		if f.TLSConfig != nil {
			tlsConfig = f.TLSConfig.Clone()
		}
		f.dohClient = &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig, ForceAttemptHTTP2: true, MaxIdleConns: 16, MaxIdleConnsPerHost: 8, MaxConnsPerHost: 32, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: f.Timeout}, Timeout: f.Timeout}
	})
	return f.dohClient
}

func withoutOPT(records []wire.RR) []wire.RR {
	out := make([]wire.RR, 0, len(records))
	for _, rr := range records {
		if rr.Header().Rrtype != wire.TypeOPT {
			out = append(out, rr)
		}
	}
	return out
}
func sameQuestion(a, b *wire.Msg) bool {
	return len(a.Question) == 1 && len(b.Question) == 1 && strings.EqualFold(a.Question[0].Name, b.Question[0].Name) && a.Question[0].Qtype == b.Question[0].Qtype && a.Question[0].Qclass == b.Question[0].Qclass
}
