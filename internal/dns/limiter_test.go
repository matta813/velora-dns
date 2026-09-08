package dns

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wire "github.com/miekg/dns"
)

func TestLimiterRefillGlobalAndClientBounds(t *testing.T) {
	limiter := NewLimiter(2, 2, 3, 3, 2)
	now := limiter.global.last
	limiter.now = func() time.Time { return now }
	a := netip.MustParseAddr("192.0.2.1")
	b := netip.MustParseAddr("192.0.2.2")
	first, second, third := limiter.Allow(a), limiter.Allow(a), limiter.Allow(a)
	if first != "" || second != "" || third != "client_rate" {
		t.Fatal("per-client burst not enforced")
	}
	if limiter.Allow(b) != "" || limiter.Allow(b) != "global_rate" {
		t.Fatal("global burst not enforced")
	}
	now = now.Add(time.Second)
	if limiter.Allow(a) != "" {
		t.Fatal("tokens did not refill")
	}
	if reason := limiter.Allow(netip.MustParseAddr("192.0.2.3")); reason != "" || len(limiter.clients) != 2 {
		t.Fatalf("bounded client replacement: %q %d", reason, len(limiter.clients))
	}
}

func TestLimiterMemoryAndConcurrencyRemainBounded(t *testing.T) {
	limiter := NewLimiter(1000, 1000, 1000, 1000, 32)
	now := limiter.global.last
	limiter.now = func() time.Time { return now }
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := range 10000 {
		wg.Go(func() {
			ip := netip.MustParseAddr(fmt.Sprintf("2001:db8::%x", i+1))
			if limiter.Allow(ip) == "" {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	if len(limiter.clients) != 32 || accepted.Load() != 1000 {
		t.Fatalf("state escaped bound: clients=%d accepted=%d", len(limiter.clients), accepted.Load())
	}
}

type transportCounts struct {
	opened, closed, rejected atomic.Int32
}

func (c *transportCounts) Overload(reason string) {
	if reason == "tcp_connections" {
		c.rejected.Add(1)
	}
}
func (c *transportCounts) TCPConnection(open bool) {
	if open {
		c.opened.Add(1)
	} else {
		c.closed.Add(1)
	}
}

func TestTCPConnectionLimitRejectsAndReleases(t *testing.T) {
	counts := &transportCounts{}
	s, err := StartWithLimits([]string{"127.0.0.1:0"}, wire.HandlerFunc(answer), 1, counts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Shutdown(t.Context()) }()
	first, err := net.Dial("tcp", s.Addresses()[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	deadline := time.Now().Add(time.Second)
	for counts.opened.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	second, err := net.Dial("tcp", s.Addresses()[0])
	if err != nil {
		t.Fatal(err)
	}
	_ = second.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = second.Read(make([]byte, 1))
	_ = second.Close()
	if counts.rejected.Load() != 1 {
		t.Fatalf("rejections=%d", counts.rejected.Load())
	}
	_ = first.Close()
	deadline = time.Now().Add(time.Second)
	for counts.closed.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	third, err := net.Dial("tcp", s.Addresses()[0])
	if err != nil {
		t.Fatal(err)
	}
	_ = third.Close()
	if counts.opened.Load() < 2 {
		t.Fatal("released slot was not reusable")
	}
}
