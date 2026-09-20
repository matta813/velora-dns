package dhcp

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestParsePacket(t *testing.T) {
	p := &Packet{
		OpCode: 1,
		HType:  1,
		HLen:   6,
		XID:    0x12345678,
		CHAddr: net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
		Options: map[byte][]byte{
			OptMessageType: {MsgDiscover},
		},
	}
	data := p.Marshal()
	got, err := ParsePacket(data)
	if err != nil {
		t.Fatalf("ParsePacket: %v", err)
	}
	if got.OpCode != 1 {
		t.Errorf("OpCode = %d, want 1", got.OpCode)
	}
	if got.XID != 0x12345678 {
		t.Errorf("XID = %d, want 0x12345678", got.XID)
	}
	if got.MessageType() != MsgDiscover {
		t.Errorf("MessageType = %d, want %d", got.MessageType(), MsgDiscover)
	}
	if got.CHAddr.String() != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("CHAddr = %s, want aa:bb:cc:dd:ee:ff", got.CHAddr)
	}
}

func TestPacketTooShort(t *testing.T) {
	_, err := ParsePacket([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for short packet")
	}
}

func TestOfferPacket(t *testing.T) {
	discover := &Packet{
		OpCode: 1,
		HType:  1,
		HLen:   6,
		XID:    42,
		CHAddr: net.HardwareAddr{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
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
	if offer.MessageType() != MsgOffer {
		t.Errorf("MessageType = %d, want %d", offer.MessageType(), MsgOffer)
	}
	if !offer.YIAddr.Equal(offeredIP) {
		t.Errorf("YIAddr = %s, want %s", offer.YIAddr, offeredIP)
	}
	if offer.XID != 42 {
		t.Errorf("XID = %d, want 42", offer.XID)
	}

	data := offer.Marshal()
	got, err := ParsePacket(data)
	if err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if got.MessageType() != MsgOffer {
		t.Errorf("round-trip MessageType = %d, want %d", got.MessageType(), MsgOffer)
	}
}

func TestPoolAllocator(t *testing.T) {
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
	mac := "aa:bb:cc:dd:ee:ff"
	ip, ok := pa.AllocateForDiscover(mac, netip.Addr{})
	if !ok {
		t.Fatal("AllocateForDiscover failed")
	}
	if !pool.Subnet.Contains(ip) {
		t.Errorf("allocated IP %s outside subnet %s", ip, pool.Subnet)
	}
}

func TestPoolAllocatorReservation(t *testing.T) {
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
	reservations := []Reservation{
		{PoolID: 1, MACAddress: "aa:bb:cc:dd:ee:ff", IPAddress: "192.168.1.50"},
	}
	pa := NewPoolAllocator(pool, reservations, nil)
	ip, ok := pa.AllocateForDiscover("aa:bb:cc:dd:ee:ff", netip.Addr{})
	if !ok {
		t.Fatal("AllocateForDiscover failed for reserved MAC")
	}
	if ip.String() != "192.168.1.50" {
		t.Errorf("got %s, want 192.168.1.50", ip)
	}
}

func TestPoolAllocatorExhaustion(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.252/30")
	pool := &Pool{
		ID:           1,
		Name:         "tiny",
		Interface:    "eth0",
		Subnet:       prefix,
		Gateway:      netip.MustParseAddr("192.168.1.253"),
		LeaseSeconds: 60,
		Enabled:      true,
	}
	pa := NewPoolAllocator(pool, nil, nil)
	// .254 is the only usable host in a /30 with .253 as gateway
	ip1, ok1 := pa.AllocateForDiscover("aa:bb:cc:dd:ee:01", netip.Addr{})
	if !ok1 {
		t.Fatal("first allocation should succeed")
	}
	_ = ip1
	_, ok2 := pa.AllocateForDiscover("aa:bb:cc:dd:ee:02", netip.Addr{})
	if ok2 {
		t.Error("second allocation should fail (pool exhausted)")
	}
}

func TestValidatePool(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.0/24")
	valid := &Pool{
		Name:         "lan",
		Interface:    "eth0",
		Subnet:       prefix,
		Gateway:      netip.MustParseAddr("192.168.1.1"),
		LeaseSeconds: 86400,
	}
	if err := ValidatePool(valid); err != nil {
		t.Errorf("valid pool: %v", err)
	}
	bad := &Pool{Name: "", Interface: "eth0", Subnet: prefix, Gateway: netip.MustParseAddr("192.168.1.1"), LeaseSeconds: 86400}
	if err := ValidatePool(bad); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestValidateReservation(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.0/24")
	pool := &Pool{Subnet: prefix, Gateway: netip.MustParseAddr("192.168.1.1")}
	if err := ValidateReservation(pool, netip.MustParseAddr("192.168.1.50")); err != nil {
		t.Errorf("valid reservation: %v", err)
	}
	if err := ValidateReservation(pool, netip.MustParseAddr("10.0.0.1")); err == nil {
		t.Error("expected error for out-of-range IP")
	}
	if err := ValidateReservation(pool, netip.MustParseAddr("192.168.1.0")); err == nil {
		t.Error("expected error for network address")
	}
	if err := ValidateReservation(pool, netip.MustParseAddr("192.168.1.255")); err == nil {
		t.Error("expected error for broadcast address")
	}
}

func TestLeaseExpiry(t *testing.T) {
	l := &Lease{
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if !l.IsExpired() {
		t.Error("lease should be expired")
	}
	l2 := &Lease{
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if l2.IsExpired() {
		t.Error("lease should not be expired")
	}
}

func TestRelease(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.0/24")
	pool := &Pool{
		ID:      1,
		Subnet:  prefix,
		Gateway: netip.MustParseAddr("192.168.1.1"),
	}
	leases := []Lease{
		{PoolID: 1, MACAddress: "aa:bb:cc:dd:ee:ff", IPAddress: "192.168.1.10", Status: LeaseActive, ExpiresAt: time.Now().Add(time.Hour)},
	}
	pa := NewPoolAllocator(pool, nil, leases)
	if !pa.Release("aa:bb:cc:dd:ee:ff") {
		t.Error("release should succeed")
	}
	if pa.Release("aa:bb:cc:dd:ee:ff") {
		t.Error("second release should fail")
	}
}
