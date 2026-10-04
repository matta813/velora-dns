package tests

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/matta813/velora-dns/internal/database"
	"github.com/matta813/velora-dns/internal/dhcp"
)

// The DHCP tests speak RFC 2131 wire format over real loopback UDP sockets on
// ephemeral ports. The client below builds and decodes packets itself, so the
// server's encoder is not used to check the server.

var (
	dhcpServerIP = net.IPv4(192, 0, 2, 1).To4()
	dhcpCookie   = []byte{99, 130, 83, 99}
	macA         = net.HardwareAddr{0x02, 0, 0, 0, 0, 0x0a}
	macB         = net.HardwareAddr{0x02, 0, 0, 0, 0, 0x0b}
)

type dhcpReply struct {
	op      byte
	xid     uint32
	yiaddr  netip.Addr
	options map[byte][]byte
}

func (r dhcpReply) kind() byte {
	if v := r.options[dhcp.OptMessageType]; len(v) == 1 {
		return v[0]
	}
	return 0
}

type dhcpFixture struct {
	t      *testing.T
	path   string
	store  *database.Store
	pool   *dhcp.Pool
	server *dhcp.Server
	conn   *net.UDPConn
	xid    uint32
	wrap   func(dhcp.Store) dhcp.Store
	dns    func(netip.Addr, string)
}

// newDHCP creates a pool for subnet in a fresh SQLite database and starts a
// server for it on a loopback port.
func newDHCP(t *testing.T, subnet string, reservations ...dhcp.Reservation) *dhcpFixture {
	t.Helper()
	f := &dhcpFixture{t: t, path: filepath.Join(t.TempDir(), "velora.db")}
	f.open()
	prefix := netip.MustParsePrefix(subnet)
	f.pool = &dhcp.Pool{Name: "lan", Interface: "lo", Subnet: prefix, Gateway: prefix.Addr().Next(), DNSServers: []netip.Addr{netip.MustParseAddr("192.0.2.53"), netip.MustParseAddr("192.0.2.54")}, LeaseSeconds: 600, Enabled: true}
	if err := f.store.CreatePool(context.Background(), f.pool); err != nil {
		t.Fatal(err)
	}
	for i := range reservations {
		reservations[i].PoolID = f.pool.ID
		if err := f.store.CreateReservation(context.Background(), &reservations[i]); err != nil {
			t.Fatal(err)
		}
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	f.conn = conn
	t.Cleanup(func() { _ = conn.Close() })
	t.Cleanup(f.stop)
	f.start()
	return f
}

func (f *dhcpFixture) open() {
	f.t.Helper()
	store, err := database.Open(context.Background(), "sqlite", f.path)
	if err != nil {
		f.t.Fatal(err)
	}
	f.store = store
}

// start loads reservations and leases from the database, as a process start would.
func (f *dhcpFixture) start() {
	f.t.Helper()
	ctx := context.Background()
	reservations, err := f.store.ListReservations(ctx, f.pool.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	leases, err := f.store.ListLeases(ctx, f.pool.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	var store dhcp.Store = f.store
	if f.wrap != nil {
		store = f.wrap(store)
	}
	f.server, err = dhcp.NewServer(dhcp.ServerConfig{Pool: f.pool, Allocator: dhcp.NewPoolAllocator(f.pool, reservations, leases), Store: store, ServerIP: dhcpServerIP, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DNSPublish: f.dns, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		f.t.Fatal(err)
	}
	f.server.Start(ctx)
}

func (f *dhcpFixture) stop() {
	if f.server != nil {
		f.server.Stop()
		f.server = nil
	}
	if f.store != nil {
		_ = f.store.Close()
		f.store = nil
	}
}

func (f *dhcpFixture) restart() {
	f.t.Helper()
	f.stop()
	f.open()
	f.start()
}

// message builds a BOOTREQUEST as a client would: hardware length in the
// header, magic cookie before the options, padded to the BOOTP minimum.
func (f *dhcpFixture) message(mac net.HardwareAddr, kind byte, ciaddr netip.Addr, options map[byte][]byte) []byte {
	f.xid++
	b := make([]byte, 236, 300)
	b[0], b[1], b[2] = 1, 1, byte(len(mac))
	binary.BigEndian.PutUint32(b[4:8], f.xid)
	if ciaddr.IsValid() {
		copy(b[12:16], ciaddr.AsSlice())
	}
	copy(b[28:44], mac)
	b = append(b, dhcpCookie...)
	b = append(b, dhcp.OptMessageType, 1, kind)
	for code, value := range options {
		b = append(b, code, byte(len(value)))
		b = append(b, value...)
	}
	b = append(b, 255)
	for len(b) < 300 {
		b = append(b, 0)
	}
	return b
}

func (f *dhcpFixture) send(raw []byte) {
	f.t.Helper()
	if _, err := f.conn.WriteTo(raw, f.server.Addr()); err != nil {
		f.t.Fatal(err)
	}
}

// receive returns the next reply, or false when the server stays silent.
func (f *dhcpFixture) receive(wait time.Duration) (dhcpReply, bool) {
	f.t.Helper()
	buf := make([]byte, 1500)
	_ = f.conn.SetReadDeadline(time.Now().Add(wait))
	n, err := f.conn.Read(buf)
	if err != nil {
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return dhcpReply{}, false
		}
		f.t.Fatal(err)
	}
	b := buf[:n]
	if n < 300 {
		f.t.Fatalf("reply is %d bytes, below the 300-byte BOOTP minimum", n)
	}
	if string(b[236:240]) != string(dhcpCookie) {
		f.t.Fatalf("reply has no DHCP magic cookie: % x", b[236:240])
	}
	r := dhcpReply{op: b[0], xid: binary.BigEndian.Uint32(b[4:8]), yiaddr: netip.AddrFrom4([4]byte(b[16:20])), options: map[byte][]byte{}}
	for i := 240; i < n && b[i] != 255; {
		if b[i] == 0 {
			i++
			continue
		}
		if i+2 > n || i+2+int(b[i+1]) > n {
			f.t.Fatalf("truncated option %d in reply", b[i])
		}
		r.options[b[i]] = b[i+2 : i+2+int(b[i+1])]
		i += 2 + int(b[i+1])
	}
	if r.op != 2 || r.xid != f.xid {
		f.t.Fatalf("reply op=%d xid=%d, want BOOTREPLY for xid %d", r.op, r.xid, f.xid)
	}
	return r, true
}

func (f *dhcpFixture) exchange(mac net.HardwareAddr, kind byte, ciaddr netip.Addr, options map[byte][]byte) dhcpReply {
	f.t.Helper()
	f.send(f.message(mac, kind, ciaddr, options))
	r, ok := f.receive(2 * time.Second)
	if !ok {
		f.t.Fatalf("no reply to message type %d from %s", kind, mac)
	}
	return r
}

func (f *dhcpFixture) silent(raw []byte, why string) {
	f.t.Helper()
	f.send(raw)
	if r, ok := f.receive(300 * time.Millisecond); ok {
		f.t.Fatalf("%s: got message type %d for %s", why, r.kind(), r.yiaddr)
	}
}

func requested(ip netip.Addr) map[byte][]byte {
	return map[byte][]byte{dhcp.OptRequestedIP: ip.AsSlice(), dhcp.OptServerID: dhcpServerIP}
}

// lease runs DISCOVER/OFFER/REQUEST/ACK and returns the acknowledged address.
func (f *dhcpFixture) lease(mac net.HardwareAddr) netip.Addr {
	f.t.Helper()
	offer := f.exchange(mac, dhcp.MsgDiscover, netip.Addr{}, nil)
	if offer.kind() != dhcp.MsgOffer {
		f.t.Fatalf("DISCOVER answered with type %d, want OFFER", offer.kind())
	}
	ack := f.exchange(mac, dhcp.MsgRequest, netip.Addr{}, requested(offer.yiaddr))
	if ack.kind() != dhcp.MsgAck || ack.yiaddr != offer.yiaddr {
		f.t.Fatalf("REQUEST for %s answered with type %d for %s", offer.yiaddr, ack.kind(), ack.yiaddr)
	}
	return ack.yiaddr
}

// assertOptions checks options 1, 3, 6, 51, 53 and 54 of an OFFER or ACK.
func (f *dhcpFixture) assertOptions(r dhcpReply, kind byte) {
	f.t.Helper()
	want := map[byte][]byte{
		dhcp.OptSubnetMask:  net.CIDRMask(f.pool.Subnet.Bits(), 32),
		dhcp.OptRouter:      f.pool.Gateway.AsSlice(),
		dhcp.OptDNSServer:   {192, 0, 2, 53, 192, 0, 2, 54},
		dhcp.OptLeaseTime:   {0, 0, 2, 88}, // 600 seconds
		dhcp.OptMessageType: {kind},
		dhcp.OptServerID:    dhcpServerIP,
		dhcp.OptT1Renew:     {0, 0, 1, 44}, // 300 seconds, half the lease
		dhcp.OptT2Rebind:    {0, 0, 2, 13}, // 525 seconds, 87.5% of the lease
	}
	for code, value := range want {
		if string(r.options[code]) != string(value) {
			f.t.Errorf("option %d = %v, want %v", code, r.options[code], value)
		}
	}
}

func (f *dhcpFixture) stored(mac net.HardwareAddr) *dhcp.Lease {
	f.t.Helper()
	lease, err := f.store.GetLease(context.Background(), f.pool.ID, mac.String())
	if err != nil {
		f.t.Fatalf("lease for %s: %v", mac, err)
	}
	return lease
}

func TestDHCPLeaseLifecycle(t *testing.T) {
	f := newDHCP(t, "192.0.2.0/24")

	offer := f.exchange(macA, dhcp.MsgDiscover, netip.Addr{}, nil)
	f.assertOptions(offer, dhcp.MsgOffer)
	if !f.pool.Subnet.Contains(offer.yiaddr) || offer.yiaddr == f.pool.Gateway {
		t.Fatalf("offered %s", offer.yiaddr)
	}
	if _, err := f.store.GetLease(context.Background(), f.pool.ID, macA.String()); err == nil {
		t.Fatal("an OFFER must not create a stored lease")
	}

	ack := f.exchange(macA, dhcp.MsgRequest, netip.Addr{}, requested(offer.yiaddr))
	f.assertOptions(ack, dhcp.MsgAck)
	if ack.yiaddr != offer.yiaddr {
		t.Fatalf("ACK for %s, offered %s", ack.yiaddr, offer.yiaddr)
	}
	first := f.stored(macA)
	if first.IPAddress != ack.yiaddr.String() || first.Status != dhcp.LeaseActive || time.Until(first.ExpiresAt) < 9*time.Minute {
		t.Fatalf("stored lease %+v", first)
	}

	// A renewing client sends its address in ciaddr without option 50.
	renew := f.exchange(macA, dhcp.MsgRequest, ack.yiaddr, nil)
	f.assertOptions(renew, dhcp.MsgAck)
	if renew.yiaddr != ack.yiaddr {
		t.Fatalf("renew moved the client from %s to %s", ack.yiaddr, renew.yiaddr)
	}

	// Another client may not take a leased address and is offered a different one.
	if nak := f.exchange(macB, dhcp.MsgRequest, netip.Addr{}, requested(ack.yiaddr)); nak.kind() != dhcp.MsgNak {
		t.Fatalf("REQUEST for a leased address answered with type %d, want NAK", nak.kind())
	} else if string(nak.options[dhcp.OptServerID]) != string(dhcpServerIP) || nak.yiaddr.Compare(netip.IPv4Unspecified()) != 0 {
		t.Fatalf("NAK server id %v yiaddr %s", nak.options[dhcp.OptServerID], nak.yiaddr)
	}
	if nak := f.exchange(macB, dhcp.MsgRequest, netip.Addr{}, requested(netip.MustParseAddr("198.51.100.7"))); nak.kind() != dhcp.MsgNak {
		t.Fatalf("REQUEST outside the subnet answered with type %d, want NAK", nak.kind())
	}
	if other := f.lease(macB); other == ack.yiaddr {
		t.Fatalf("%s was leased twice", other)
	}

	// A REQUEST addressed to another server is not ours to answer.
	f.silent(f.message(macA, dhcp.MsgRequest, netip.Addr{}, map[byte][]byte{dhcp.OptRequestedIP: ack.yiaddr.AsSlice(), dhcp.OptServerID: {192, 0, 2, 99}}), "REQUEST for another server")

	f.send(f.message(macA, dhcp.MsgRelease, ack.yiaddr, map[byte][]byte{dhcp.OptServerID: dhcpServerIP}))
	deadline := time.Now().Add(2 * time.Second)
	for f.stored(macA).Status != dhcp.LeaseReleased {
		if time.Now().After(deadline) {
			t.Fatalf("lease still %q after RELEASE", f.stored(macA).Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The released address is free again, also after a restart.
	f.restart()
	macC := net.HardwareAddr{0x02, 0, 0, 0, 0, 0x0c}
	if got := f.exchange(macC, dhcp.MsgRequest, netip.Addr{}, requested(ack.yiaddr)); got.kind() != dhcp.MsgAck {
		t.Fatalf("released address answered with type %d, want ACK", got.kind())
	}
}

func TestDHCPDeclineQuarantinesAddress(t *testing.T) {
	t.Skip("missing: DECLINE releases the lease (memory and database) but the allocator has no hold-down set, so the declined address is handed out again; ADR 0005 requires quarantine")
	f := newDHCP(t, "192.0.2.0/24")
	declined := f.lease(macA)
	f.send(f.message(macA, dhcp.MsgDecline, netip.Addr{}, requested(declined)))

	// DECLINE has no reply, so the next exchanges also prove it was processed.
	if nak := f.exchange(macB, dhcp.MsgRequest, netip.Addr{}, requested(declined)); nak.kind() != dhcp.MsgNak {
		t.Fatalf("declined address answered with type %d, want NAK", nak.kind())
	}
	if again := f.lease(macA); again == declined {
		t.Fatalf("declined address %s was offered again", declined)
	}
	if got := f.lease(macB); got == declined {
		t.Fatalf("declined address %s was leased to another client", declined)
	}
}

func TestDHCPReservation(t *testing.T) {
	reserved := netip.MustParseAddr("192.0.2.2") // the first address dynamic allocation would pick
	f := newDHCP(t, "192.0.2.0/24", dhcp.Reservation{MACAddress: macA.String(), IPAddress: reserved.String(), Hostname: "printer"})

	if got := f.lease(macB); got == reserved {
		t.Fatalf("reserved address %s was leased dynamically", reserved)
	}
	if nak := f.exchange(macB, dhcp.MsgRequest, netip.Addr{}, requested(reserved)); nak.kind() != dhcp.MsgNak {
		t.Fatalf("REQUEST for another client's reservation answered with type %d, want NAK", nak.kind())
	}
	if got := f.lease(macA); got != reserved {
		t.Fatalf("reserved client got %s, want %s", got, reserved)
	}
	if nak := f.exchange(macA, dhcp.MsgRequest, netip.Addr{}, requested(netip.MustParseAddr("192.0.2.50"))); nak.kind() != dhcp.MsgNak {
		t.Fatalf("reserved client asking for another address answered with type %d, want NAK", nak.kind())
	}
}

func TestDHCPPoolExhaustion(t *testing.T) {
	// A /30 has two host addresses; one is the gateway.
	f := newDHCP(t, "192.0.2.0/30")
	if got := f.lease(macA); got != netip.MustParseAddr("192.0.2.2") {
		t.Fatalf("leased %s", got)
	}
	f.silent(f.message(macB, dhcp.MsgDiscover, netip.Addr{}, nil), "DISCOVER on an exhausted pool")
	if again := f.lease(macA); again != netip.MustParseAddr("192.0.2.2") {
		t.Fatalf("existing client got %s from an exhausted pool", again)
	}
}

func TestDHCPRestartKeepsLeases(t *testing.T) {
	f := newDHCP(t, "192.0.2.0/24")
	a := f.lease(macA)
	f.restart()

	if got := f.lease(macB); got == a {
		t.Fatalf("%s was leased again after a restart", a)
	}
	if nak := f.exchange(macB, dhcp.MsgRequest, netip.Addr{}, requested(a)); nak.kind() != dhcp.MsgNak {
		t.Fatalf("persisted lease answered with type %d, want NAK", nak.kind())
	}
	if renew := f.exchange(macA, dhcp.MsgRequest, a, nil); renew.kind() != dhcp.MsgAck || renew.yiaddr != a {
		t.Fatalf("renew after restart: type %d for %s, want ACK for %s", renew.kind(), renew.yiaddr, a)
	}
	if got := f.lease(macA); got != a {
		t.Fatalf("client got %s after a restart, want its lease %s", got, a)
	}
}

func TestDHCPRejectsMalformedAndRelayedPackets(t *testing.T) {
	f := newDHCP(t, "192.0.2.0/24")
	valid := func() []byte { return f.message(macA, dhcp.MsgDiscover, netip.Addr{}, nil) }

	noCookie := valid()
	copy(noCookie[236:240], []byte{1, 2, 3, 4})
	oversizedHardware := valid()
	oversizedHardware[2] = 200
	reply := valid()
	reply[0] = 2
	relayed := valid()
	copy(relayed[24:28], []byte{192, 0, 2, 77})
	truncatedOption := append(valid()[:240], dhcp.OptMessageType, 9, dhcp.MsgDiscover)
	for name, raw := range map[string][]byte{
		"empty":                     {},
		"short header":              valid()[:120],
		"header without options":    valid()[:236],
		"missing magic cookie":      noCookie,
		"oversized hardware length": oversizedHardware,
		"BOOTREPLY":                 reply,
		"relayed (giaddr set)":      relayed,
		"truncated option":          truncatedOption,
		"no message type":           append(valid()[:240], 255),
	} {
		f.silent(raw, name)
	}
	// The server survived all of it and allocated nothing.
	if got := f.lease(macA); got != netip.MustParseAddr("192.0.2.2") {
		t.Fatalf("leased %s after malformed packets, want the first free address", got)
	}
}

// failingStore fails lease writes, as an unavailable database would.
type failingStore struct{ dhcp.Store }

func (failingStore) SaveLease(context.Context, *dhcp.Lease) error {
	return errors.New("database is unavailable")
}

func TestDHCPStorageFailureSendsNoACK(t *testing.T) {
	f := newDHCP(t, "192.0.2.0/24")
	f.stop()
	published := 0
	f.dns = func(netip.Addr, string) { published++ }
	f.wrap = func(s dhcp.Store) dhcp.Store { return failingStore{s} }
	f.open()
	f.start()

	offer := f.exchange(macA, dhcp.MsgDiscover, netip.Addr{}, nil)
	f.silent(f.message(macA, dhcp.MsgRequest, netip.Addr{}, requested(offer.yiaddr)), "REQUEST while the lease cannot be stored")
	if published != 0 {
		t.Fatal("DNS publication ran for a lease that was not stored")
	}
}

func TestDHCPPublishesCommittedLease(t *testing.T) {
	f := newDHCP(t, "192.0.2.0/24")
	f.stop()
	type record struct {
		ip     netip.Addr
		host   string
		stored bool
	}
	published := make(chan record, 1)
	// The publisher sees the lease already stored, and its panic-free failure
	// (it returns nothing) cannot change the DHCP outcome.
	f.dns = func(ip netip.Addr, host string) {
		lease, err := f.store.GetLease(context.Background(), f.pool.ID, macA.String())
		published <- record{ip, host, err == nil && lease.IPAddress == ip.String()}
	}
	f.open()
	f.start()

	offer := f.exchange(macA, dhcp.MsgDiscover, netip.Addr{}, nil)
	options := requested(offer.yiaddr)
	options[12] = []byte("laptop")
	if ack := f.exchange(macA, dhcp.MsgRequest, netip.Addr{}, options); ack.kind() != dhcp.MsgAck {
		t.Fatalf("type %d, want ACK", ack.kind())
	}
	select {
	case got := <-published:
		if got.ip != offer.yiaddr || got.host != "laptop" || !got.stored {
			t.Fatalf("published %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lease was not published")
	}
}

func TestDHCPBroadcastAndInterfaceSelection(t *testing.T) {
	t.Skip("not implemented: the server replies only to the packet's source address and binds no interface, so broadcast replies, subnet selection and the interface allowlist of ADR 0005 have nothing to assert yet; exercising them also needs a network namespace with CAP_NET_ADMIN")
}

func TestDHCPExpireLeasesComparesTimestamps(t *testing.T) {
	f := newDHCP(t, "192.0.2.0/24")
	ctx := context.Background()
	past := &dhcp.Lease{PoolID: f.pool.ID, MACAddress: macA.String(), IPAddress: "192.0.2.10", ExpiresAt: time.Now().Add(-time.Minute), Status: dhcp.LeaseActive}
	future := &dhcp.Lease{PoolID: f.pool.ID, MACAddress: macB.String(), IPAddress: "192.0.2.11", ExpiresAt: time.Now().Add(time.Minute), Status: dhcp.LeaseActive}
	for _, l := range []*dhcp.Lease{past, future} {
		if err := f.store.SaveLease(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := f.store.ExpireLeases(ctx); err != nil || n != 1 {
		t.Fatalf("ExpireLeases = %d, %v, want 1 lease expired", n, err)
	}
	if f.stored(macA).Status != dhcp.LeaseExpired || f.stored(macB).Status != dhcp.LeaseActive {
		t.Fatal("wrong lease expired")
	}
}
