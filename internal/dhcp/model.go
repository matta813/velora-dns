// Package dhcp implements a DHCPv4 server with lease management, pool
// allocation, and optional DNS record publication.
package dhcp

import (
	"fmt"
	"net"
	"net/netip"
	"time"
)

// Message types (RFC 2131 section 4.3).
const (
	MsgDiscover byte = 1
	MsgOffer    byte = 2
	MsgRequest  byte = 3
	MsgDecline  byte = 4
	MsgAck      byte = 5
	MsgNak      byte = 6
	MsgRelease  byte = 7
	MsgInform   byte = 8
)

// DHCP options (RFC 2131 and RFC 3397).
const (
	OptSubnetMask      byte = 1
	OptRouter          byte = 3
	OptDNSServer       byte = 6
	OptDomainName      byte = 15
	OptRequestedIP     byte = 50
	OptLeaseTime       byte = 51
	OptMessageType     byte = 53
	OptServerID        byte = 54
	OptRequestedParams byte = 55
	OptClientID        byte = 61
	OptT1Renew         byte = 58
	OptT2Rebind        byte = 59
)

// LeaseStatus represents the lifecycle state of a DHCP lease.
type LeaseStatus string

const (
	LeaseActive   LeaseStatus = "active"
	LeaseExpired  LeaseStatus = "expired"
	LeaseReleased LeaseStatus = "released"
)

// Pool represents a DHCP address pool bound to a network interface.
type Pool struct {
	ID           int64        `json:"id"`
	Name         string       `json:"name"`
	Interface    string       `json:"interface"`
	Subnet       netip.Prefix `json:"subnet"`
	Gateway      netip.Addr   `json:"gateway"`
	DNSServers   []netip.Addr `json:"dns_servers"`
	LeaseSeconds int          `json:"lease_seconds"`
	Enabled      bool         `json:"enabled"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// Reservation maps a MAC address to a fixed IP within a pool.
type Reservation struct {
	ID         int64  `json:"id"`
	PoolID     int64  `json:"pool_id"`
	MACAddress string `json:"mac_address"`
	IPAddress  string `json:"ip_address"`
	Hostname   string `json:"hostname"`
}

// Lease records an active, expired, or released DHCP lease.
type Lease struct {
	ID         int64       `json:"id"`
	PoolID     int64       `json:"pool_id"`
	MACAddress string      `json:"mac_address"`
	IPAddress  string      `json:"ip_address"`
	Hostname   string      `json:"hostname"`
	ClientID   string      `json:"client_id"`
	ExpiresAt  time.Time   `json:"expires_at"`
	Status     LeaseStatus `json:"status"`
	CreatedAt  time.Time   `json:"created_at"`
}

// IsExpired reports whether the lease has passed its expiry time.
func (l *Lease) IsExpired() bool {
	return time.Now().After(l.ExpiresAt)
}

// ValidatePool checks that a pool configuration is internally consistent.
func ValidatePool(p *Pool) error {
	if p.Name == "" {
		return fmt.Errorf("pool name is required")
	}
	if p.Interface == "" {
		return fmt.Errorf("pool interface is required")
	}
	if !p.Subnet.IsValid() {
		return fmt.Errorf("invalid subnet")
	}
	if !p.Gateway.IsValid() {
		return fmt.Errorf("invalid gateway")
	}
	if !p.Subnet.Contains(p.Gateway) {
		return fmt.Errorf("gateway %s is outside subnet %s", p.Gateway, p.Subnet)
	}
	if p.LeaseSeconds < 60 || p.LeaseSeconds > 31536000 {
		return fmt.Errorf("lease_seconds must be between 60 and 31536000")
	}
	return nil
}

// ValidateReservation checks that an IP is within the pool subnet and not the
// network or broadcast address.
func ValidateReservation(pool *Pool, ip netip.Addr) error {
	if !pool.Subnet.Contains(ip) {
		return fmt.Errorf("reservation IP %s is outside pool subnet %s", ip, pool.Subnet)
	}
	ones, bits := pool.Subnet.Bits(), net.IPv4len*8
	if bits == 32 && ip == networkAddr(pool.Subnet) {
		return fmt.Errorf("reservation IP must not be the network address")
	}
	if bits == 32 && ip == broadcastAddr(pool.Subnet) {
		return fmt.Errorf("reservation IP must not be the broadcast address")
	}
	_ = ones
	return nil
}

func networkAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().As4()
	ones := p.Bits()
	if remainder := ones % 8; remainder != 0 {
		b[ones/8] &= 0xFF << (8 - remainder)
	}
	for i := ones/8 + 1; i < 4; i++ {
		b[i] = 0
	}
	return netip.AddrFrom4(b)
}

func broadcastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().As4()
	ones := p.Bits()
	for i := ones / 8; i < 4; i++ {
		b[i] = 0xFF
	}
	if remainder := ones % 8; remainder != 0 {
		b[ones/8] = p.Addr().As4()[ones/8] | (0xFF >> remainder)
	}
	return netip.AddrFrom4(b)
}
