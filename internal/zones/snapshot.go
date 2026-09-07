package zones

import (
	"strings"

	wire "github.com/miekg/dns"
)

type view struct {
	zone    Zone
	records map[string][]wire.RR
	exists  map[string]bool
}
type snapshot struct {
	byID   map[int64]Zone
	byName map[string]*view
}

func parent(name string) string {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return ""
}
func (s *snapshot) find(name string) *view {
	for name != "" {
		if v := s.byName[name]; v != nil {
			return v
		}
		name = parent(name)
	}
	return nil
}
func compile(all []Zone) (*snapshot, error) {
	if len(all) > MaxZones {
		return nil, invalid("maximum zone count reached")
	}
	s := &snapshot{byID: map[int64]Zone{}, byName: map[string]*view{}}
	total := 0
	for _, input := range all {
		z, err := normalize(input)
		if err != nil {
			return nil, err
		}
		if _, ok := s.byName[z.Name]; ok {
			return nil, ErrExists
		}
		total += len(z.Records)
		if total > MaxTotalRecords {
			return nil, invalid("maximum total record count reached")
		}
		v := &view{zone: z, records: map[string][]wire.RR{}, exists: map[string]bool{z.Name: true}}
		ids := map[int64]bool{}
		unique := map[string]bool{}
		for _, r := range z.Records {
			if r.ID < 0 || r.ID > 0 && ids[r.ID] {
				return nil, invalid("invalid or duplicate record ID")
			}
			ids[r.ID] = true
			rr, err := recordRR(r)
			if err != nil {
				return nil, err
			}
			key := rr.String()
			if unique[key] {
				return nil, invalid("duplicate record")
			}
			unique[key] = true
			for _, existing := range v.records[r.Name] {
				if existing.Header().Rrtype == wire.TypeCNAME || rr.Header().Rrtype == wire.TypeCNAME {
					return nil, invalid("CNAME cannot coexist with other records")
				}
				if existing.Header().Rrtype == rr.Header().Rrtype && existing.Header().Ttl != r.TTL {
					return nil, invalid("records in one RRset must use the same TTL")
				}
			}
			v.records[r.Name] = append(v.records[r.Name], rr)
			for name := r.Name; wire.IsSubDomain(z.Name, name) && name != ""; name = parent(name) {
				v.exists[name] = true
			}
		}
		s.byID[z.ID] = z
		s.byName[z.Name] = v
	}
	// Reject loops within and across local zones, respecting the closest zone boundary.
	for _, v := range s.byName {
		for name, records := range v.records {
			for _, rr := range records {
				if _, ok := rr.(*wire.CNAME); !ok {
					continue
				}
				seen := map[string]bool{}
				target := name
				for depth := 0; ; depth++ {
					if seen[target] || depth >= 16 {
						return nil, invalid("CNAME loop or chain longer than 16 names")
					}
					seen[target] = true
					owner := s.find(target)
					if owner == nil {
						break
					}
					next := ""
					for _, r := range owner.records[target] {
						if c, ok := r.(*wire.CNAME); ok {
							next = c.Target
							break
						}
					}
					if next == "" {
						break
					}
					target = next
				}
			}
		}
	}
	return s, nil
}
func (v *view) soa() wire.RR {
	return &wire.SOA{Hdr: wire.RR_Header{Name: v.zone.Name, Rrtype: wire.TypeSOA, Class: wire.ClassINET, Ttl: 60}, Ns: v.zone.PrimaryNS, Mbox: v.zone.Contact, Serial: v.zone.Revision, Refresh: 3600, Retry: 600, Expire: 604800, Minttl: 60}
}
func (v *view) lookup(name string, kind uint16) []wire.RR {
	if name == v.zone.Name && kind == wire.TypeSOA {
		return []wire.RR{v.soa()}
	}
	var answer []wire.RR
	for _, rr := range v.records[name] {
		if rr.Header().Rrtype == kind {
			answer = append(answer, wire.Copy(rr))
		}
	}
	if name == v.zone.Name && kind == wire.TypeNS && len(answer) == 0 {
		return []wire.RR{&wire.NS{Hdr: wire.RR_Header{Name: name, Rrtype: wire.TypeNS, Class: wire.ClassINET, Ttl: 300}, Ns: v.zone.PrimaryNS}}
	}
	return answer
}
