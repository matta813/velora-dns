package dhcp

import (
	"log/slog"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.0/24")
	pool := &Pool{
		ID:           1,
		Name:         "test",
		Interface:    "eth0",
		Subnet:       prefix,
		Gateway:      netip.MustParseAddr("192.168.1.1"),
		DNSServers:   []netip.Addr{netip.MustParseAddr("1.1.1.1")},
		LeaseSeconds: 86400,
		Enabled:      true,
	}
	pa := NewPoolAllocator(pool, nil, nil)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	srv, err := NewServer(ServerConfig{
		Pool:       pool,
		Allocator:  pa,
		ServerIP:   net.IPv4(192, 168, 1, 1),
		Logger:     logger,
		ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Stop()

	ctx := t.Context()
	srv.Start(ctx)

	if srv.conn == nil {
		t.Fatal("server conn is nil")
	}
	if srv.conn.LocalAddr() == nil {
		t.Fatal("server not listening")
	}
}

func TestMakeOfferRoundTrip(t *testing.T) {
	discover := &Packet{
		OpCode: 1,
		HType:  1,
		HLen:   6,
		XID:    0xDEADBEEF,
		CHAddr: net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
		Options: map[byte][]byte{
			OptMessageType: {MsgDiscover},
		},
	}

	serverIP := net.IPv4(192, 168, 1, 1)
	offeredIP := net.IPv4(192, 168, 1, 100)
	subnet := net.IPv4(255, 255, 255, 0)
	gateway := net.IPv4(192, 168, 1, 1)
	dns := []net.IP{net.IPv4(1, 1, 1, 1)}

	offer := MakeOffer(discover, offeredIP, 86400, serverIP, subnet, gateway, dns)
	data := offer.Marshal()
	got, err := ParsePacket(data)
	if err != nil {
		t.Fatalf("parse offer: %v", err)
	}

	if got.MessageType() != MsgOffer {
		t.Errorf("MessageType = %d, want %d", got.MessageType(), MsgOffer)
	}
	if !got.YIAddr.Equal(offeredIP) {
		t.Errorf("YIAddr = %s, want %s", got.YIAddr, offeredIP)
	}
	if got.XID != 0xDEADBEEF {
		t.Errorf("XID = %08x, want DEADBEEF", got.XID)
	}
	if _, ok := got.ServerID(); !ok {
		t.Error("missing server ID")
	}
	if lt := got.LeaseTime(); lt != 86400 {
		t.Errorf("lease time = %d, want 86400", lt)
	}
}

func TestMakeNakRoundTrip(t *testing.T) {
	request := &Packet{
		OpCode: 1,
		HType:  1,
		HLen:   6,
		XID:    12345,
		CHAddr: net.HardwareAddr{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
		Options: map[byte][]byte{
			OptMessageType: {MsgRequest},
		},
	}
	serverIP := net.IPv4(192, 168, 1, 1)

	nak := MakeNak(request, serverIP)
	data := nak.Marshal()
	got, err := ParsePacket(data)
	if err != nil {
		t.Fatalf("parse nak: %v", err)
	}
	if got.MessageType() != MsgNak {
		t.Errorf("MessageType = %d, want %d", got.MessageType(), MsgNak)
	}
	if got.XID != 12345 {
		t.Errorf("XID = %d, want 12345", got.XID)
	}
}

func TestServerHandleRelease(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.0/24")
	pool := &Pool{
		ID:           1,
		Name:         "test",
		Interface:    "eth0",
		Subnet:       prefix,
		Gateway:      netip.MustParseAddr("192.168.1.1"),
		LeaseSeconds: 86400,
		Enabled:      true,
	}
	pa := NewPoolAllocator(pool, nil, nil)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := &Server{pool: pool, allocator: pa, logger: logger}

	mac := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	ip, ok := pa.AllocateForDiscover(mac.String(), netip.Addr{})
	if !ok {
		t.Fatal("initial allocation failed")
	}
	_ = ip

	release := &Packet{
		OpCode: 1,
		HType:  1,
		HLen:   6,
		XID:    99,
		CHAddr: mac,
		Options: map[byte][]byte{
			OptMessageType: {MsgRelease},
		},
	}

	srv.handleRelease(release)

	// After release, the IP should be available again.
	ip2, ok2 := pa.AllocateForDiscover(mac.String(), netip.Addr{})
	if !ok2 {
		t.Fatal("allocation after release should succeed")
	}
	if !ip2.IsValid() {
		t.Error("expected valid IP after release")
	}
}

func TestPoolAllocatorReleaseLeases(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.0/24")
	pool := &Pool{
		ID:           1,
		Name:         "test",
		Interface:    "eth0",
		Subnet:       prefix,
		Gateway:      netip.MustParseAddr("192.168.1.1"),
		LeaseSeconds: 3600,
		Enabled:      true,
	}

	leases := []Lease{
		{PoolID: 1, MACAddress: "aa:bb:cc:dd:ee:ff", IPAddress: "192.168.1.10", Status: LeaseActive, ExpiresAt: time.Now().Add(time.Hour)},
		{PoolID: 1, MACAddress: "11:22:33:44:55:66", IPAddress: "192.168.1.11", Status: LeaseActive, ExpiresAt: time.Now().Add(time.Hour)},
	}
	pa := NewPoolAllocator(pool, nil, leases)

	active := pa.ActiveLeases()
	if len(active) != 2 {
		t.Errorf("active leases = %d, want 2", len(active))
	}

	if !pa.Release("aa:bb:cc:dd:ee:ff") {
		t.Error("release should succeed")
	}

	active = pa.ActiveLeases()
	if len(active) != 1 {
		t.Errorf("active leases after release = %d, want 1", len(active))
	}
}

func TestOptionUint32RoundTrip(t *testing.T) {
	vals := []uint32{0, 1, 60, 3600, 86400, 31536000}
	for _, v := range vals {
		b := OptionUint32(v)
		if len(b) != 4 {
			t.Fatalf("OptionUint32(%d) returned %d bytes", v, len(b))
		}
		got := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		if got != v {
			t.Errorf("OptionUint32(%d) round-trip = %d", v, got)
		}
	}
}
