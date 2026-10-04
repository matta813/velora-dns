package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"
)

type Server struct {
	pool       *Pool
	allocator  *PoolAllocator
	store      Store
	serverIP   net.IP
	conn       *net.UDPConn
	logger     *slog.Logger
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	dnsPublish func(netip.Addr, string)
}

type ServerConfig struct {
	Pool       *Pool
	Allocator  *PoolAllocator
	Store      Store
	ServerIP   net.IP
	Logger     *slog.Logger
	DNSPublish func(netip.Addr, string)
	ListenAddr string // override for testing; empty uses pool default
}

func NewServer(cfg ServerConfig) (*Server, error) {
	listenAddr := cfg.ListenAddr
	if listenAddr == "" {
		listenAddr = cfg.Pool.ListenAddr()
	}
	addr, err := net.ResolveUDPAddr("udp4", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve DHCP listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("bind DHCP UDP: %w", err)
	}
	return &Server{
		pool:       cfg.Pool,
		allocator:  cfg.Allocator,
		store:      cfg.Store,
		serverIP:   cfg.ServerIP,
		conn:       conn,
		logger:     cfg.Logger,
		dnsPublish: cfg.DNSPublish,
	}, nil
}

func (s *Server) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.serve(ctx)
	}()
	s.logger.Info("dhcp server started", "listen", s.conn.LocalAddr().String())
}

func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.wg.Wait()
	s.logger.Info("dhcp server stopped")
}

func (s *Server) serve(ctx context.Context) {
	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.conn.SetReadDeadline(time.Now().Add(time.Second))
		n, remoteAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-ctx.Done():
				return
			default:
				s.logger.Warn("dhcp read error", "err", err)
				continue
			}
		}
		pkt, err := ParsePacket(buf[:n])
		if err != nil {
			s.logger.Debug("dhcp parse error", "err", err, "from", remoteAddr)
			continue
		}
		s.handle(ctx, pkt, remoteAddr)
	}
}

func (s *Server) handle(ctx context.Context, pkt *Packet, remoteAddr *net.UDPAddr) {
	if pkt.OpCode != 1 {
		return // not a BOOTREQUEST
	}
	if !pkt.GIAddr.Equal(net.IPv4zero) {
		s.logger.Debug("dhcp relayed packet dropped: relay agents are not supported", "giaddr", pkt.GIAddr)
		return
	}
	switch pkt.MessageType() {
	case MsgDiscover:
		s.handleDiscover(ctx, pkt, remoteAddr)
	case MsgRequest:
		s.handleRequest(ctx, pkt, remoteAddr)
	case MsgRelease:
		s.handleRelease(ctx, pkt)
	case MsgDecline:
		s.handleDecline(ctx, pkt)
	default:
		s.logger.Debug("unhandled dhcp message type", "type", pkt.MessageType())
	}
}

func netToNetip(ip net.IP) netip.Addr {
	if ip4 := ip.To4(); ip4 != nil {
		a, _ := netip.AddrFromSlice(ip4)
		return a
	}
	a, _ := netip.AddrFromSlice(ip)
	return a
}

func netipToNet(addr netip.Addr) net.IP {
	b := addr.As4()
	return net.IP(b[:]).To4()
}

func (s *Server) handleDiscover(ctx context.Context, pkt *Packet, remoteAddr *net.UDPAddr) {
	mac := pkt.CHAddr.String()
	reqIP, _ := pkt.RequestedIP()
	var requestedIP netip.Addr
	if reqIP != nil {
		requestedIP = netToNetip(reqIP)
	}

	ip, ok := s.allocator.AllocateForDiscover(mac, requestedIP)
	if !ok {
		s.logger.Warn("dhcp pool exhausted", "mac", mac)
		return
	}

	leaseSec := uint32(s.pool.LeaseSeconds)
	offer := MakeOffer(pkt, netipToNet(ip), leaseSec, s.serverIP,
		LeaseToSubnet(s.pool.Subnet),
		netipToNet(s.pool.Gateway),
		ipsToNet(s.pool.DNSServers))
	if _, err := s.conn.WriteToUDP(offer.Marshal(), remoteAddr); err != nil {
		s.logger.Warn("dhcp send offer", "err", err)
	}
	s.logger.Debug("dhcp offer", "mac", mac, "ip", ip, "lease", leaseSec)
}

func (s *Server) handleRequest(ctx context.Context, pkt *Packet, remoteAddr *net.UDPAddr) {
	mac := pkt.CHAddr.String()
	reqIP, _ := pkt.RequestedIP()
	var requestedIP netip.Addr
	if reqIP != nil {
		requestedIP = netToNetip(reqIP)
	}
	if !requestedIP.IsValid() && !pkt.CIAddr.Equal(net.IPv4zero) {
		requestedIP = netToNetip(pkt.CIAddr) // RENEWING/REBINDING carry the address in ciaddr
	}
	serverID, hasServerID := pkt.ServerID()

	if hasServerID && !serverID.Equal(s.serverIP) {
		return
	}

	ip, ok := s.allocator.AllocateForRequest(mac, requestedIP)
	if !ok {
		nak := MakeNak(pkt, s.serverIP)
		if _, err := s.conn.WriteToUDP(nak.Marshal(), remoteAddr); err != nil {
			s.logger.Warn("dhcp send nak", "err", err)
		}
		s.logger.Debug("dhcp nak", "mac", mac, "requested", requestedIP)
		return
	}

	leaseSec := uint32(s.pool.LeaseSeconds)
	lease := &Lease{
		PoolID:     s.pool.ID,
		MACAddress: mac,
		IPAddress:  ip.String(),
		Hostname:   pkt.Hostname(),
		ClientID:   string(pkt.ClientID()),
		ExpiresAt:  time.Now().Add(time.Duration(leaseSec) * time.Second),
		Status:     LeaseActive,
	}
	// Commit before ACK: a client must never hold an address we failed to record.
	if err := s.store.SaveLease(ctx, lease); err != nil {
		s.logger.Error("dhcp persist lease, withholding ACK", "err", err)
		return
	}

	ack := MakeAck(pkt, netipToNet(ip), leaseSec, s.serverIP,
		LeaseToSubnet(s.pool.Subnet),
		netipToNet(s.pool.Gateway),
		ipsToNet(s.pool.DNSServers))
	if _, err := s.conn.WriteToUDP(ack.Marshal(), remoteAddr); err != nil {
		s.logger.Warn("dhcp send ack", "err", err)
	}
	s.logger.Debug("dhcp ack", "mac", mac, "ip", ip, "lease", leaseSec)

	if s.dnsPublish != nil {
		s.dnsPublish(ip, pkt.Hostname())
	}
}

func (s *Server) handleRelease(ctx context.Context, pkt *Packet) {
	mac := pkt.CHAddr.String()
	if serverID, ok := pkt.ServerID(); ok && !serverID.Equal(s.serverIP) {
		return
	}
	if s.releaseLease(ctx, mac, pkt.CIAddr) {
		s.logger.Debug("dhcp release", "mac", mac)
	}
}

func (s *Server) handleDecline(ctx context.Context, pkt *Packet) {
	mac := pkt.CHAddr.String()
	ip, ok := pkt.RequestedIP()
	if !ok {
		return
	}
	s.logger.Warn("dhcp decline", "mac", mac, "ip", ip)
	// ponytail: declined addresses are released, not quarantined (ADR 0005 wants a hold-down); add a timed deny set in PoolAllocator.
	s.releaseLease(ctx, mac, ip)
}

// releaseLease frees mac's lease in memory and in the store when it holds ip.
func (s *Server) releaseLease(ctx context.Context, mac string, ip net.IP) bool {
	l, err := s.store.GetLease(ctx, s.pool.ID, mac)
	if err != nil || l.Status != LeaseActive || l.IPAddress != ip.String() {
		return false
	}
	l.Status = LeaseReleased
	if err := s.store.SaveLease(ctx, l); err != nil {
		s.logger.Error("dhcp persist release", "err", err)
		return false
	}
	s.allocator.Release(mac)
	return true
}

func ipsToNet(addrs []netip.Addr) []net.IP {
	out := make([]net.IP, len(addrs))
	for i, a := range addrs {
		out[i] = netipToNet(a)
	}
	return out
}

// Addr returns the bound listener address.
func (s *Server) Addr() net.Addr { return s.conn.LocalAddr() }

func (p *Pool) ListenAddr() string {
	return "0.0.0.0:67"
}
