package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	wire "github.com/miekg/dns"
)

// TransferState represents the state of a zone transfer.
type TransferState int

const (
	TransferIdle TransferState = iota
	TransferRunning
	TransferComplete
	TransferFailed
)

// TransferResult contains the result of a zone transfer.
type TransferResult struct {
	State    TransferState
	Records  []wire.RR
	SOA      *wire.SOA
	Errors   []error
	Duration time.Duration
}

// TransferClient performs AXFR and IXFR zone transfers.
type TransferClient struct {
	TSIGStore *TSIGStore
	Timeout   time.Duration
}

// NewTransferClient creates a new transfer client.
func NewTransferClient(tsigStore *TSIGStore, timeout time.Duration) *TransferClient {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &TransferClient{
		TSIGStore: tsigStore,
		Timeout:   timeout,
	}
}

// AXFR performs a full zone transfer (AXFR) from a primary server.
func (c *TransferClient) AXFR(ctx context.Context, zone, primaryAddr, tsigKeyName string) (*TransferResult, error) {
	result := &TransferResult{State: TransferRunning}
	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
		if len(result.Errors) > 0 {
			result.State = TransferFailed
		} else {
			result.State = TransferComplete
		}
	}()

	conn, err := net.DialTimeout("tcp", primaryAddr, c.Timeout)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("connect to primary: %w", err))
		return result, nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.Timeout))

	msg := new(wire.Msg)
	msg.SetQuestion(zone, wire.TypeSOA)
	msg.RecursionDesired = false
	msg.Question[0].Qclass = wire.ClassINET

	if c.TSIGStore != nil && tsigKeyName != "" {
		if err := c.TSIGStore.SignMessage(msg, tsigKeyName); err != nil {
			result.Errors = append(result.Errors, err)
			return result, nil
		}
	}

	tcpConn := &wire.Conn{Conn: conn}

	if err := tcpConn.WriteMsg(msg); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("send SOA query: %w", err))
		return result, nil
	}

	soaResp, err := tcpConn.ReadMsg()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("read SOA response: %w", err))
		return result, nil
	}

	if soaResp.Rcode != wire.RcodeSuccess || len(soaResp.Answer) == 0 {
		result.Errors = append(result.Errors, fmt.Errorf("SOA query failed: %s", wire.RcodeToString[soaResp.Rcode]))
		return result, nil
	}

	for _, rr := range soaResp.Answer {
		if soa, ok := rr.(*wire.SOA); ok {
			result.SOA = soa
			break
		}
	}

	axfrQuery := new(wire.Msg)
	axfrQuery.SetQuestion(zone, wire.TypeAXFR)
	axfrQuery.RecursionDesired = false

	if c.TSIGStore != nil && tsigKeyName != "" {
		if err := c.TSIGStore.SignMessage(axfrQuery, tsigKeyName); err != nil {
			result.Errors = append(result.Errors, err)
			return result, nil
		}
	}

	if err := tcpConn.WriteMsg(axfrQuery); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("send AXFR query: %w", err))
		return result, nil
	}

	soaCount := 0
	for {
		if ctx.Err() != nil {
			result.Errors = append(result.Errors, ctx.Err())
			return result, nil
		}

		response, err := tcpConn.ReadMsg()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("read AXFR response: %w", err))
			return result, nil
		}

		if response.Rcode != wire.RcodeSuccess {
			result.Errors = append(result.Errors, fmt.Errorf("AXFR failed: %s", wire.RcodeToString[response.Rcode]))
			return result, nil
		}

		for _, rr := range response.Answer {
			if rr.Header().Rrtype != wire.TypeOPT {
				result.Records = append(result.Records, rr)
				if soa, ok := rr.(*wire.SOA); ok {
					result.SOA = soa
					soaCount++
				}
			}
		}

		if soaCount >= 2 {
			break
		}
	}

	return result, nil
}

// IXFR performs an incremental zone transfer (IXFR) from a primary server.
func (c *TransferClient) IXFR(ctx context.Context, zone, primaryAddr string, serial uint32, tsigKeyName string) (*TransferResult, error) {
	result := &TransferResult{State: TransferRunning}
	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
		if len(result.Errors) > 0 {
			result.State = TransferFailed
		} else {
			result.State = TransferComplete
		}
	}()

	conn, err := net.DialTimeout("tcp", primaryAddr, c.Timeout)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("connect to primary: %w", err))
		return result, nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.Timeout))

	msg := new(wire.Msg)
	msg.SetQuestion(zone, wire.TypeIXFR)
	msg.RecursionDesired = false
	msg.Question[0].Qclass = wire.ClassINET

	soa := &wire.SOA{
		Hdr:    wire.RR_Header{Name: zone, Rrtype: wire.TypeSOA, Class: wire.ClassINET, Ttl: 0},
		Serial: serial,
	}
	msg.Ns = []wire.RR{soa}

	if c.TSIGStore != nil && tsigKeyName != "" {
		if err := c.TSIGStore.SignMessage(msg, tsigKeyName); err != nil {
			result.Errors = append(result.Errors, err)
			return result, nil
		}
	}

	tcpConn := &wire.Conn{Conn: conn}

	if err := tcpConn.WriteMsg(msg); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("send IXFR query: %w", err))
		return result, nil
	}

	for {
		if ctx.Err() != nil {
			result.Errors = append(result.Errors, ctx.Err())
			return result, nil
		}

		response, err := tcpConn.ReadMsg()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("read IXFR response: %w", err))
			return result, nil
		}

		if response.Rcode != wire.RcodeSuccess {
			result.Errors = append(result.Errors, fmt.Errorf("IXFR failed: %s", wire.RcodeToString[response.Rcode]))
			return result, nil
		}

		for _, rr := range response.Answer {
			if rr.Header().Rrtype != wire.TypeOPT {
				result.Records = append(result.Records, rr)
				if soa, ok := rr.(*wire.SOA); ok && result.SOA == nil {
					result.SOA = soa
				}
			}
		}

		if response.Truncated {
			continue
		}

		break
	}

	return result, nil
}

// TransferHandler handles incoming AXFR/IXFR requests.
type TransferHandler struct {
	TSIGStore   *TSIGStore
	ZoneManager ZoneManager
	AllowedNets []net.IPNet
}

// ZoneManager is the interface for zone data access during transfers.
type ZoneManager interface {
	GetZoneByName(name string) ([]wire.RR, error)
	GetZoneSOA(name string) (*wire.SOA, error)
}

// ServeAXFR handles an incoming AXFR request.
func (h *TransferHandler) ServeAXFR(w wire.ResponseWriter, q *wire.Msg) {
	if len(q.Question) != 1 {
		m := new(wire.Msg)
		m.SetRcode(q, wire.RcodeFormatError)
		_ = w.WriteMsg(m)
		return
	}

	zone := q.Question[0].Name

	if !h.isAllowed(w.RemoteAddr()) {
		m := new(wire.Msg)
		m.SetRcode(q, wire.RcodeRefused)
		_ = w.WriteMsg(m)
		return
	}

	if h.TSIGStore != nil {
		if q.IsTsig() == nil {
			m := new(wire.Msg)
			m.SetRcode(q, wire.RcodeRefused)
			_ = w.WriteMsg(m)
			return
		}
		if _, err := h.TSIGStore.VerifyMessage(q); err != nil {
			m := new(wire.Msg)
			m.SetRcode(q, wire.RcodeRefused)
			_ = w.WriteMsg(m)
			return
		}
	}

	records, err := h.ZoneManager.GetZoneByName(zone)
	if err != nil {
		m := new(wire.Msg)
		m.SetRcode(q, wire.RcodeServerFailure)
		_ = w.WriteMsg(m)
		return
	}

	soa, err := h.ZoneManager.GetZoneSOA(zone)
	if err != nil {
		m := new(wire.Msg)
		m.SetRcode(q, wire.RcodeServerFailure)
		_ = w.WriteMsg(m)
		return
	}

	response := new(wire.Msg)
	response.SetReply(q)
	response.Authoritative = true
	response.Answer = make([]wire.RR, 0, len(records)+2)
	response.Answer = append(response.Answer, soa)
	response.Answer = append(response.Answer, records...)
	response.Answer = append(response.Answer, soa)

	_ = w.WriteMsg(response)
}

func (h *TransferHandler) isAllowed(addr net.Addr) bool {
	if len(h.AllowedNets) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, allowed := range h.AllowedNets {
		if allowed.Contains(ip) {
			return true
		}
	}
	return false
}

// ParseTransferAddress parses a transfer address (host:port or just host).
func ParseTransferAddress(addr string) string {
	if !strings.Contains(addr, ":") {
		return addr + ":53"
	}
	return addr
}
