package zones

import (
	"fmt"
	"net"
	"strings"
	"unicode/utf8"

	wire "github.com/miekg/dns"
)

func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalid, message) }
func domain(value string) (string, error) {
	value = strings.ToLower(strings.TrimSuffix(value, "."))
	if value == "" || len(value) > 253 {
		return "", invalid("name must contain 1–253 ASCII characters")
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", invalid("invalid domain label")
		}
		for _, r := range label {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789-_", r) {
				return "", invalid("names must use ASCII labels without wildcards or escapes")
			}
		}
	}
	return value + ".", nil
}
func normalize(z Zone) (Zone, error) {
	var err error
	z.Name, err = domain(z.Name)
	if err != nil {
		return z, err
	}
	if z.PrimaryNS == "" {
		z.PrimaryNS = "ns." + z.Name
	}
	if z.Contact == "" {
		z.Contact = "hostmaster." + z.Name
	}
	z.PrimaryNS, err = domain(z.PrimaryNS)
	if err != nil {
		return z, err
	}
	z.Contact, err = domain(z.Contact)
	if err != nil {
		return z, err
	}
	if len(z.Records) > MaxRecords {
		return z, invalid("zone exceeds 1000 records")
	}
	z = clone(z)
	for i, record := range z.Records {
		if record.Name == "@" || record.Name == "" {
			record.Name = z.Name
		} else if !strings.HasSuffix(record.Name, ".") {
			record.Name += "." + z.Name
		}
		record.Name, err = domain(record.Name)
		if err != nil {
			return z, err
		}
		if !wire.IsSubDomain(z.Name, record.Name) {
			return z, invalid("record name is outside its zone")
		}
		record.Type = strings.ToUpper(record.Type)
		if record.TTL > 86400 {
			return z, invalid("TTL must be between 0 and 86400 seconds")
		}
		if record.Type != "MX" && record.Priority != 0 {
			return z, invalid("priority is only valid for MX records")
		}
		switch record.Type {
		case "CNAME", "MX", "NS", "PTR":
			record.Value, err = domain(record.Value)
			if err != nil {
				return z, err
			}
		case "A", "AAAA":
			ip := net.ParseIP(record.Value)
			if ip == nil {
				return z, invalid("invalid IP address")
			}
			record.Value = ip.String()
		}
		if record.Type == "CNAME" && record.Name == z.Name {
			return z, invalid("zone apex cannot contain a CNAME")
		}
		if record.Type == "NS" && record.Name != z.Name {
			return z, invalid("only apex NS records are supported; delegations are not implemented")
		}
		if _, err = recordRR(record); err != nil {
			return z, err
		}
		z.Records[i] = record
	}
	return z, nil
}
func recordRR(r Record) (wire.RR, error) {
	h := wire.RR_Header{Name: r.Name, Class: wire.ClassINET, Ttl: r.TTL}
	h.Rrtype = wire.StringToType[r.Type]
	switch r.Type {
	case "A":
		ip := net.ParseIP(r.Value)
		if ip == nil || ip.To4() == nil {
			return nil, invalid("A requires an IPv4 address")
		}
		return &wire.A{Hdr: h, A: ip.To4()}, nil
	case "AAAA":
		ip := net.ParseIP(r.Value)
		if ip == nil || ip.To4() != nil {
			return nil, invalid("AAAA requires an IPv6 address")
		}
		return &wire.AAAA{Hdr: h, AAAA: ip}, nil
	case "CNAME":
		return &wire.CNAME{Hdr: h, Target: r.Value}, nil
	case "MX":
		return &wire.MX{Hdr: h, Mx: r.Value, Preference: r.Priority}, nil
	case "NS":
		return &wire.NS{Hdr: h, Ns: r.Value}, nil
	case "PTR":
		return &wire.PTR{Hdr: h, Ptr: r.Value}, nil
	case "TXT":
		if len(r.Value) > 2048 || !utf8.ValidString(r.Value) {
			return nil, invalid("TXT requires valid UTF-8 of at most 2048 bytes")
		}
		value := r.Value
		parts := []string{}
		for len(value) > 255 {
			n := 255
			for !utf8.RuneStart(value[n]) {
				n--
			}
			parts = append(parts, value[:n])
			value = value[n:]
		}
		parts = append(parts, value)
		return &wire.TXT{Hdr: h, Txt: parts}, nil
	default:
		return nil, invalid("unsupported record type")
	}
}
