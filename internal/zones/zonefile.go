package zones

import (
	"fmt"
	"io"
	"strings"

	wire "github.com/miekg/dns"
)

// ParseZoneFile accepts one IN zone with an SOA and record types supported by
// Velora. Callers can safely persist the returned zone as one atomic revision.
func ParseZoneFile(input string) (Zone, error) {
	p := wire.NewZoneParser(strings.NewReader(input), "", "import.zone")
	var z Zone
	for rr, ok := p.Next(); ok; rr, ok = p.Next() {
		h := rr.Header()
		if h.Class != wire.ClassINET {
			return z, invalid("only IN records are supported")
		}
		if soa, ok := rr.(*wire.SOA); ok {
			if z.Name != "" {
				return z, invalid("zone file must contain exactly one SOA")
			}
			z.Name, z.PrimaryNS, z.Contact = h.Name, soa.Ns, soa.Mbox
			continue
		}
		r, err := importRecord(rr)
		if err != nil {
			return z, err
		}
		z.Records = append(z.Records, r)
	}
	if err := p.Err(); err != nil {
		return z, invalid(fmt.Sprintf("invalid zone file: %v", err))
	}
	if z.Name == "" {
		return z, invalid("zone file requires one SOA record")
	}
	return normalize(z)
}

func importRecord(rr wire.RR) (Record, error) {
	h := rr.Header()
	r := Record{Name: h.Name, TTL: h.Ttl}
	switch value := rr.(type) {
	case *wire.A:
		r.Type, r.Value = "A", value.A.String()
	case *wire.AAAA:
		r.Type, r.Value = "AAAA", value.AAAA.String()
	case *wire.CNAME:
		r.Type, r.Value = "CNAME", value.Target
	case *wire.MX:
		r.Type, r.Value, r.Priority = "MX", value.Mx, value.Preference
	case *wire.NS:
		r.Type, r.Value = "NS", value.Ns
	case *wire.PTR:
		r.Type, r.Value = "PTR", value.Ptr
	case *wire.TXT:
		r.Type, r.Value = "TXT", strings.Join(value.Txt, "")
	default:
		return r, invalid(fmt.Sprintf("unsupported zone-file record type %s", wire.TypeToString[h.Rrtype]))
	}
	return r, nil
}

// FormatZoneFile serializes the zone's SOA and supported records in standard
// presentation format. It is intentionally not a database backup format.
func FormatZoneFile(z Zone) (string, error) {
	z, err := normalize(z)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	soa := &wire.SOA{Hdr: wire.RR_Header{Name: z.Name, Rrtype: wire.TypeSOA, Class: wire.ClassINET, Ttl: 3600}, Ns: z.PrimaryNS, Mbox: z.Contact, Serial: z.Revision, Refresh: 3600, Retry: 600, Expire: 86400, Minttl: 300}
	if z.Revision == 0 {
		soa.Serial = 1
	}
	out.WriteString(soa.String())
	out.WriteByte('\n')
	for _, record := range z.Records {
		rr, err := recordRR(record)
		if err != nil {
			return "", err
		}
		out.WriteString(rr.String())
		out.WriteByte('\n')
	}
	return out.String(), nil
}

func ReadZoneFile(r io.Reader, limit int64) (Zone, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return Zone{}, err
	}
	if int64(len(data)) > limit {
		return Zone{}, invalid("zone file exceeds size limit")
	}
	return ParseZoneFile(string(data))
}
