package dhcp

import (
	"encoding/binary"
	"fmt"
	"net"
)

// Packet represents a parsed DHCPv4 message (RFC 2131 section 2).
type Packet struct {
	OpCode    byte
	HType     byte
	HLen      byte
	Hops      byte
	XID       uint32
	Secs      uint16
	Flags     uint16
	CIAddr    net.IP
	YIAddr    net.IP
	SIAddr    net.IP
	GIAddr    net.IP
	CHAddr    net.HardwareAddr
	SName     string
	File      string
	Options   map[byte][]byte
	RelayAddr net.IP
}

const (
	headerFixedLen = 236
	optionEndByte  = 255
	optionPadByte  = 0
)

// ParsePacket decodes a raw DHCP UDP payload into a Packet.
func ParsePacket(data []byte) (*Packet, error) {
	if len(data) < headerFixedLen {
		return nil, fmt.Errorf("dhcp packet too short: %d bytes", len(data))
	}
	p := &Packet{
		OpCode: data[0],
		HType:  data[1],
		HLen:   data[2],
		Hops:   data[3],
		XID:    binary.BigEndian.Uint32(data[4:8]),
		Secs:   binary.BigEndian.Uint16(data[8:10]),
		Flags:  binary.BigEndian.Uint16(data[10:12]),
		CIAddr: net.IP(data[12:16]).To4(),
		YIAddr: net.IP(data[16:20]).To4(),
		SIAddr: net.IP(data[20:24]).To4(),
		GIAddr: net.IP(data[24:28]).To4(),
		CHAddr: make(net.HardwareAddr, data[43]),
	}
	copy(p.CHAddr, data[28:28+data[43]])
	// SName: 64 bytes starting at offset 44
	sNameRaw := data[44:108]
	if i := findNul(sNameRaw); i >= 0 {
		p.SName = string(sNameRaw[:i])
	} else {
		p.SName = string(sNameRaw)
	}
	// File: 128 bytes starting at offset 108
	fileRaw := data[108:236]
	if i := findNul(fileRaw); i >= 0 {
		p.File = string(fileRaw[:i])
	} else {
		p.File = string(fileRaw)
	}
	p.Options = parseOptions(data[headerFixedLen:])
	p.RelayAddr = net.IP(data[24:28]).To4()
	return p, nil
}

// Marshal serializes a Packet into wire format.
func (p *Packet) Marshal() []byte {
	buf := make([]byte, headerFixedLen)
	buf[0] = p.OpCode
	buf[1] = p.HType
	buf[2] = p.HLen
	buf[3] = p.Hops
	binary.BigEndian.PutUint32(buf[4:8], p.XID)
	binary.BigEndian.PutUint16(buf[8:10], p.Secs)
	binary.BigEndian.PutUint16(buf[10:12], p.Flags)
	copy(buf[12:16], p.CIAddr.To4())
	copy(buf[16:20], p.YIAddr.To4())
	copy(buf[20:24], p.SIAddr.To4())
	copy(buf[24:28], p.GIAddr.To4())
	hLen := len(p.CHAddr)
	if hLen > 16 {
		hLen = 16
	}
	buf[43] = byte(hLen)
	copy(buf[28:28+hLen], p.CHAddr)
	optBuf := marshalOptions(p.Options)
	return append(buf, optBuf...)
}

// SetOption adds or replaces a DHCP option.
func (p *Packet) SetOption(code byte, value []byte) {
	if p.Options == nil {
		p.Options = make(map[byte][]byte)
	}
	p.Options[code] = value
}

// GetOption returns the value for a DHCP option, or nil if not present.
func (p *Packet) GetOption(code byte) []byte {
	if p.Options == nil {
		return nil
	}
	return p.Options[code]
}

// MessageType returns the DHCP message type from option 53.
func (p *Packet) MessageType() byte {
	v := p.GetOption(OptMessageType)
	if len(v) < 1 {
		return 0
	}
	return v[0]
}

// SetMessageType sets the DHCP message type option.
func (p *Packet) SetMessageType(msgType byte) {
	p.SetOption(OptMessageType, []byte{msgType})
}

// RequestedIP returns the requested IP from option 50.
func (p *Packet) RequestedIP() (net.IP, bool) {
	v := p.GetOption(OptRequestedIP)
	if len(v) < 4 {
		return nil, false
	}
	return net.IP(v[:4]).To4(), true
}

// ServerID returns the server identifier from option 54.
func (p *Packet) ServerID() (net.IP, bool) {
	v := p.GetOption(OptServerID)
	if len(v) < 4 {
		return nil, false
	}
	return net.IP(v[:4]).To4(), true
}

// ClientID returns the client identifier from option 61.
func (p *Packet) ClientID() []byte {
	return p.GetOption(OptClientID)
}

// Hostname returns the hostname from the CHAddr or the hostname option if present.
func (p *Packet) Hostname() string {
	v := p.GetOption(12) // Hostname option
	if len(v) > 0 {
		if i := findNul(v); i >= 0 {
			return string(v[:i])
		}
		return string(v)
	}
	return ""
}

// LeaseTime returns the requested lease time from option 51.
func (p *Packet) LeaseTime() uint32 {
	v := p.GetOption(OptLeaseTime)
	if len(v) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(v[:4])
}

func parseOptions(data []byte) map[byte][]byte {
	opts := make(map[byte][]byte)
	for i := 0; i < len(data); {
		if data[i] == optionEndByte {
			break
		}
		if data[i] == optionPadByte {
			i++
			continue
		}
		if i+1 >= len(data) {
			break
		}
		code := data[i]
		length := int(data[i+1])
		i += 2
		if i+length > len(data) {
			break
		}
		val := make([]byte, length)
		copy(val, data[i:i+length])
		opts[code] = val
		i += length
	}
	return opts
}

func marshalOptions(opts map[byte][]byte) []byte {
	if len(opts) == 0 {
		return []byte{optionEndByte}
	}
	var buf []byte
	for code, val := range opts {
		buf = append(buf, code, byte(len(val)))
		buf = append(buf, val...)
	}
	buf = append(buf, optionEndByte)
	return buf
}

func findNul(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return -1
}

// MakeOffer builds an OFFER response packet from a DISCOVER.
func MakeOffer(discover *Packet, offeredIP net.IP, leaseSec uint32, serverIP net.IP, subnet net.IP, gateway net.IP, dnsServers []net.IP) *Packet {
	return buildResponse(discover, MsgOffer, offeredIP, leaseSec, serverIP, subnet, gateway, dnsServers)
}

// MakeAck builds an ACK or NAK response packet.
func MakeAck(request *Packet, ackIP net.IP, leaseSec uint32, serverIP net.IP, subnet net.IP, gateway net.IP, dnsServers []net.IP) *Packet {
	return buildResponse(request, MsgAck, ackIP, leaseSec, serverIP, subnet, gateway, dnsServers)
}

// MakeNak builds a NAK response packet.
func MakeNak(request *Packet, serverIP net.IP) *Packet {
	p := &Packet{
		OpCode:  2, // BOOTREPLY
		HType:   request.HType,
		HLen:    request.HLen,
		XID:     request.XID,
		CIAddr:  request.CIAddr,
		CHAddr:  request.CHAddr,
		SIAddr:  serverIP,
		Options: map[byte][]byte{},
	}
	p.SetMessageType(MsgNak)
	p.SetOption(OptServerID, serverIP.To4())
	return p
}

func buildResponse(in *Packet, msgType byte, yiAddr net.IP, leaseSec uint32, serverIP net.IP, subnet net.IP, gateway net.IP, dnsServers []net.IP) *Packet {
	p := &Packet{
		OpCode: 2, // BOOTREPLY
		HType:  in.HType,
		HLen:   in.HLen,
		XID:    in.XID,
		CHAddr: in.CHAddr,
		SIAddr: serverIP,
		Options: map[byte][]byte{
			OptSubnetMask: subnet.To4(),
			OptRouter:     gateway.To4(),
			OptServerID:   serverIP.To4(),
		},
	}
	p.SetMessageType(msgType)
	p.YIAddr = yiAddr
	if leaseSec > 0 {
		leaseBytes := make([]byte, 4)
		leaseBytes[0] = byte(leaseSec >> 24)
		leaseBytes[1] = byte(leaseSec >> 16)
		leaseBytes[2] = byte(leaseSec >> 8)
		leaseBytes[3] = byte(leaseSec)
		p.SetOption(OptLeaseTime, leaseBytes)
		p.SetOption(OptT1Renew, leaseBytes)  // T1 = 50% of lease
		p.SetOption(OptT2Rebind, leaseBytes) // T2 = 87.5% of lease
	}
	if len(dnsServers) > 0 {
		var dnsBytes []byte
		for _, dns := range dnsServers {
			dnsBytes = append(dnsBytes, dns.To4()...)
		}
		p.SetOption(OptDNSServer, dnsBytes)
	}
	return p
}
