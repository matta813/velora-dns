package dhcp

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

// PoolAllocator manages IP address allocation within a subnet.
// It tracks active leases, reservations, and available addresses.
type PoolAllocator struct {
	pool         *Pool
	reservations map[string]netip.Addr // MAC -> reserved IP
	leases       map[string]*Lease     // MAC -> lease
	ips          map[netip.Addr]string // IP -> MAC (active leases)
	mu           sync.Mutex
}

// NewPoolAllocator creates a pool allocator seeded with existing leases and reservations.
func NewPoolAllocator(pool *Pool, reservations []Reservation, leases []Lease) *PoolAllocator {
	pa := &PoolAllocator{
		pool:         pool,
		reservations: make(map[string]netip.Addr),
		leases:       make(map[string]*Lease),
		ips:          make(map[netip.Addr]string),
	}
	for _, r := range reservations {
		ip, err := netip.ParseAddr(r.IPAddress)
		if err == nil {
			pa.reservations[r.MACAddress] = ip
		}
	}
	for i := range leases {
		l := &leases[i]
		if l.Status == LeaseActive && !l.IsExpired() {
			ip, err := netip.ParseAddr(l.IPAddress)
			if err == nil {
				pa.leases[l.MACAddress] = l
				pa.ips[ip] = l.MACAddress
			}
		}
	}
	return pa
}

// AllocateForDiscover finds an available IP for a DISCOVER message.
// It checks reservations first, then allocates from the pool range.
func (pa *PoolAllocator) AllocateForDiscover(mac string, requestedIP netip.Addr) (netip.Addr, bool) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	// Check reservation.
	if reserved, ok := pa.reservations[mac]; ok {
		// Verify the reservation IP is not in use by another MAC.
		if owner, ok := pa.ips[reserved]; !ok || owner == mac {
			return reserved, true
		}
		// Reservation IP is in use; fall through to dynamic allocation.
	}

	// If client requested a specific IP and it's available, honor it.
	if requestedIP.IsValid() {
		if owner, ok := pa.ips[requestedIP]; !ok || owner == mac {
			if pa.pool.Subnet.Contains(requestedIP) && !isNetworkOrBroadcast(requestedIP, pa.pool.Subnet) {
				return requestedIP, true
			}
		}
	}

	// Allocate from pool range.
	return pa.allocateFromRange(mac)
}

// AllocateForRequest confirms a previously offered IP or allocates a new one.
func (pa *PoolAllocator) AllocateForRequest(mac string, requestedIP netip.Addr) (netip.Addr, bool) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	// Check reservation.
	if reserved, ok := pa.reservations[mac]; ok {
		if !requestedIP.IsValid() || requestedIP == reserved {
			if owner, ok := pa.ips[reserved]; !ok || owner == mac {
				return reserved, true
			}
		}
		// Reservation mismatch; NAK handled by caller.
		return netip.Addr{}, false
	}

	// For REQUEST after DISCOVER, the requested IP should match what we offered.
	if requestedIP.IsValid() {
		if owner, ok := pa.ips[requestedIP]; !ok || owner == mac {
			if pa.pool.Subnet.Contains(requestedIP) && !isNetworkOrBroadcast(requestedIP, pa.pool.Subnet) {
				return requestedIP, true
			}
		}
		// IP conflict or out of range.
		return netip.Addr{}, false
	}

	// Allocate from pool range.
	return pa.allocateFromRange(mac)
}

// Release removes a lease for the given MAC address.
func (pa *PoolAllocator) Release(mac string) bool {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	l, ok := pa.leases[mac]
	if !ok {
		return false
	}
	ip, err := netip.ParseAddr(l.IPAddress)
	if err == nil {
		delete(pa.ips, ip)
	}
	delete(pa.leases, mac)
	return true
}

// ActiveLeases returns a snapshot of all active leases.
func (pa *PoolAllocator) ActiveLeases() []Lease {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	var out []Lease
	for _, l := range pa.leases {
		if !l.IsExpired() {
			out = append(out, *l)
		}
	}
	return out
}

// ExpireLeases marks expired leases and returns them for cleanup.
func (pa *PoolAllocator) ExpireLeases() []Lease {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	var expired []Lease
	now := time.Now()
	for mac, l := range pa.leases {
		if now.After(l.ExpiresAt) {
			l.Status = LeaseExpired
			expired = append(expired, *l)
			ip, err := netip.ParseAddr(l.IPAddress)
			if err == nil {
				delete(pa.ips, ip)
			}
			delete(pa.leases, mac)
		}
	}
	return expired
}

func (pa *PoolAllocator) allocateFromRange(mac string) (netip.Addr, bool) {
	start, end := poolRange(pa.pool.Subnet)
	for ip := start; ip != end; ip = nextIP(ip) {
		if isNetworkOrBroadcast(ip, pa.pool.Subnet) {
			continue
		}
		if _, ok := pa.ips[ip]; ok {
			continue
		}
		// Skip gateway.
		if ip == pa.pool.Gateway {
			continue
		}
		pa.ips[ip] = mac
		pa.leases[mac] = &Lease{
			PoolID:     pa.pool.ID,
			MACAddress: mac,
			IPAddress:  ip.String(),
			Status:     LeaseActive,
		}
		return ip, true
	}
	return netip.Addr{}, false // Pool exhausted
}

func poolRange(subnet netip.Prefix) (netip.Addr, netip.Addr) {
	start := networkAddr(subnet)
	end := broadcastAddr(subnet)
	return nextIP(start), end
}

func nextIP(ip netip.Addr) netip.Addr {
	if ip4 := ip.As4(); ip4 != [4]byte{} {
		ip4[3]++
		if ip4[3] == 0 {
			ip4[2]++
			if ip4[2] == 0 {
				ip4[1]++
				if ip4[1] == 0 {
					ip4[0]++
				}
			}
		}
		return netip.AddrFrom4(ip4)
	}
	return ip
}

func isNetworkOrBroadcast(ip netip.Addr, subnet netip.Prefix) bool {
	return ip == networkAddr(subnet) || ip == broadcastAddr(subnet)
}

// LeaseToSubnet returns the subnet mask as a net.IP for DHCP option encoding.
func LeaseToSubnet(prefix netip.Prefix) net.IP {
	ones := prefix.Bits()
	mask := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		if ones >= 8 {
			mask[i] = 0xFF
			ones -= 8
		} else if ones > 0 {
			mask[i] = 0xFF << (8 - uint(ones))
			ones = 0
		}
	}
	return mask
}

// IPFromBytes converts 4 bytes to net.IP.
func IPFromBytes(b []byte) net.IP {
	if len(b) < 4 {
		return nil
	}
	return net.IP(b[:4]).To4()
}

// OptionUint32 encodes a uint32 as 4 bytes big-endian.
func OptionUint32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// ValidateMAC checks that a MAC address string is valid.
func ValidateMAC(mac string) error {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return fmt.Errorf("invalid MAC address %q: %w", mac, err)
	}
	if len(hw) != 6 {
		return fmt.Errorf("MAC address must be 6 bytes, got %d", len(hw))
	}
	return nil
}
