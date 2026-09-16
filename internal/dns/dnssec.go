package dns

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	wire "github.com/miekg/dns"
)

type DNSSECStatus uint8

const (
	DNSSECInsecure DNSSECStatus = iota
	DNSSECSecure
)

type DNSSECValidator struct {
	anchors map[string][]*wire.DS
	now     func() time.Time
}

type dnssecExchange func(context.Context, *wire.Msg) (*wire.Msg, error)

func NewDNSSECValidator(records []string) (*DNSSECValidator, error) {
	v := &DNSSECValidator{anchors: make(map[string][]*wire.DS), now: time.Now}
	for _, record := range records {
		rr, err := wire.NewRR(record)
		if err != nil {
			return nil, fmt.Errorf("parse trust anchor: %w", err)
		}
		ds, ok := rr.(*wire.DS)
		if !ok {
			return nil, fmt.Errorf("trust anchor is not a DS record")
		}
		name := wire.CanonicalName(ds.Hdr.Name)
		v.anchors[name] = append(v.anchors[name], ds)
	}
	if len(v.anchors) == 0 {
		return nil, errors.New("at least one DS trust anchor is required")
	}
	return v, nil
}

func (v *DNSSECValidator) Validate(ctx context.Context, q, response *wire.Msg, exchange dnssecExchange) (DNSSECStatus, error) {
	if response.Rcode != wire.RcodeSuccess && response.Rcode != wire.RcodeNameError {
		return DNSSECInsecure, nil
	}
	signer := responseSigner(response)
	if signer == "" {
		return DNSSECInsecure, nil
	}
	keys, secure, err := v.validatedKeys(ctx, signer, exchange, make(map[string]bool))
	if err != nil || !secure {
		return DNSSECInsecure, err
	}
	if err = v.validateMessage(q, response, keys); err != nil {
		return DNSSECInsecure, err
	}
	return DNSSECSecure, nil
}

func (v *DNSSECValidator) validatedKeys(ctx context.Context, zone string, exchange dnssecExchange, visiting map[string]bool) ([]*wire.DNSKEY, bool, error) {
	zone = wire.CanonicalName(zone)
	if visiting[zone] {
		return nil, false, errors.New("DNSSEC delegation loop")
	}
	visiting[zone] = true
	defer delete(visiting, zone)

	keyResponse, err := dnssecQuery(ctx, zone, wire.TypeDNSKEY, exchange)
	if err != nil {
		return nil, false, err
	}
	keys := dnskeys(keyResponse.Answer, zone)
	if len(keys) == 0 {
		return nil, false, errors.New("secure zone returned no DNSKEY")
	}
	trustedDS := v.anchors[zone]
	if len(trustedDS) == 0 {
		dsResponse, queryErr := dnssecQuery(ctx, zone, wire.TypeDS, exchange)
		if queryErr != nil {
			return nil, false, queryErr
		}
		parentSigner := responseSigner(dsResponse)
		if parentSigner == "" || strings.EqualFold(parentSigner, zone) {
			return nil, false, nil
		}
		parentKeys, parentSecure, keyErr := v.validatedKeys(ctx, parentSigner, exchange, visiting)
		if keyErr != nil || !parentSecure {
			return nil, false, keyErr
		}
		if err = v.validateMessage(question(zone, wire.TypeDS), dsResponse, parentKeys); err != nil {
			return nil, false, fmt.Errorf("validate DS for %s: %w", zone, err)
		}
		trustedDS = dsRecords(dsResponse.Answer, zone)
		if len(trustedDS) == 0 {
			return nil, false, nil
		}
	}
	trustedKeys := matchDS(keys, trustedDS)
	if len(trustedKeys) == 0 {
		return nil, false, errors.New("DNSKEY does not match trusted DS")
	}
	if err = verifyRRSet(keyResponse.Answer, zone, wire.TypeDNSKEY, trustedKeys, v.now()); err != nil {
		return nil, false, fmt.Errorf("validate DNSKEY for %s: %w", zone, err)
	}
	return keys, true, nil
}

func (v *DNSSECValidator) validateMessage(q, response *wire.Msg, keys []*wire.DNSKEY) error {
	sets := make(map[string][]wire.RR)
	for _, section := range [][]wire.RR{response.Answer, response.Ns} {
		for _, rr := range section {
			t := rr.Header().Rrtype
			if t == wire.TypeRRSIG || t == wire.TypeOPT || t == wire.TypeNS {
				continue
			}
			key := wire.CanonicalName(rr.Header().Name) + fmt.Sprintf("/%d", t)
			sets[key] = append(sets[key], rr)
		}
	}
	for _, set := range sets {
		if err := verifyRRSet(append(response.Answer, response.Ns...), set[0].Header().Name, set[0].Header().Rrtype, keys, v.now()); err != nil {
			return err
		}
	}
	if response.Rcode == wire.RcodeNameError || (response.Rcode == wire.RcodeSuccess && len(response.Answer) == 0) {
		return validateDenial(q.Question[0], response)
	}
	return nil
}

func verifyRRSet(records []wire.RR, name string, kind uint16, keys []*wire.DNSKEY, now time.Time) error {
	var set []wire.RR
	var signatures []*wire.RRSIG
	for _, rr := range records {
		if strings.EqualFold(rr.Header().Name, name) && rr.Header().Rrtype == kind {
			set = append(set, rr)
		}
		if sig, ok := rr.(*wire.RRSIG); ok && strings.EqualFold(sig.Hdr.Name, name) && sig.TypeCovered == kind {
			signatures = append(signatures, sig)
		}
	}
	for _, sig := range signatures {
		if !sig.ValidityPeriod(now) {
			continue
		}
		for _, key := range keys {
			if sig.Verify(key, set) == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("missing or invalid RRSIG for %s %s", name, wire.TypeToString[kind])
}

func validateDenial(q wire.Question, response *wire.Msg) error {
	var nsecs []*wire.NSEC
	var nsec3s []*wire.NSEC3
	for _, rr := range response.Ns {
		switch proof := rr.(type) {
		case *wire.NSEC:
			nsecs = append(nsecs, proof)
			if strings.EqualFold(proof.Hdr.Name, q.Name) {
				if response.Rcode == wire.RcodeSuccess && !containsType(proof.TypeBitMap, q.Qtype) {
					return nil
				}
			}
		case *wire.NSEC3:
			nsec3s = append(nsec3s, proof)
			if response.Rcode == wire.RcodeSuccess && proof.Match(q.Name) && !containsType(proof.TypeBitMap, q.Qtype) {
				return nil
			}
		}
	}
	if response.Rcode == wire.RcodeNameError && (validNSECNXDomain(q.Name, nsecs) || validNSEC3NXDomain(q.Name, nsec3s)) {
		return nil
	}
	return errors.New("negative response lacks authenticated denial")
}

func validNSECNXDomain(name string, proofs []*wire.NSEC) bool {
	closest := closestEncloser(name, func(candidate string) bool {
		for _, proof := range proofs {
			if strings.EqualFold(proof.Hdr.Name, candidate) {
				return true
			}
		}
		return false
	})
	if closest == "" {
		return false
	}
	wildcard := "*." + closest
	return coveredByNSEC(proofs, name) && coveredByNSEC(proofs, wildcard)
}

func validNSEC3NXDomain(name string, proofs []*wire.NSEC3) bool {
	closest := closestEncloser(name, func(candidate string) bool {
		for _, proof := range proofs {
			if proof.Match(candidate) {
				return true
			}
		}
		return false
	})
	if closest == "" {
		return false
	}
	nextCloser := name
	labels := wire.SplitDomainName(name)
	closestLabels := wire.CountLabel(closest)
	if len(labels) > closestLabels {
		nextCloser = strings.Join(labels[len(labels)-closestLabels-1:], ".") + "."
	}
	return coveredByNSEC3(proofs, nextCloser) && coveredByNSEC3(proofs, "*."+closest)
}

func closestEncloser(name string, exists func(string) bool) string {
	labels := wire.SplitDomainName(name)
	for i := 1; i <= len(labels); i++ {
		candidate := strings.Join(labels[i:], ".") + "."
		if i == len(labels) {
			candidate = "."
		}
		if exists(candidate) {
			return candidate
		}
	}
	return ""
}

func coveredByNSEC(proofs []*wire.NSEC, name string) bool {
	for _, proof := range proofs {
		if nsecCovers(proof, name) {
			return true
		}
	}
	return false
}

func coveredByNSEC3(proofs []*wire.NSEC3, name string) bool {
	for _, proof := range proofs {
		if proof.Cover(name) {
			return true
		}
	}
	return false
}

func nsecCovers(nsec *wire.NSEC, name string) bool {
	owner := wire.CanonicalName(nsec.Hdr.Name)
	next := wire.CanonicalName(nsec.NextDomain)
	target := wire.CanonicalName(name)
	if owner < next {
		return owner < target && target < next
	}
	return target > owner || target < next
}

func containsType(types []uint16, target uint16) bool {
	for _, kind := range types {
		if kind == target {
			return true
		}
	}
	return false
}

func responseSigner(response *wire.Msg) string {
	for _, section := range [][]wire.RR{response.Answer, response.Ns} {
		for _, rr := range section {
			if sig, ok := rr.(*wire.RRSIG); ok {
				return wire.CanonicalName(sig.SignerName)
			}
		}
	}
	return ""
}

func dnskeys(records []wire.RR, name string) []*wire.DNSKEY {
	var out []*wire.DNSKEY
	for _, rr := range records {
		if key, ok := rr.(*wire.DNSKEY); ok && strings.EqualFold(key.Hdr.Name, name) {
			out = append(out, key)
		}
	}
	return out
}

func dsRecords(records []wire.RR, name string) []*wire.DS {
	var out []*wire.DS
	for _, rr := range records {
		if ds, ok := rr.(*wire.DS); ok && strings.EqualFold(ds.Hdr.Name, name) {
			out = append(out, ds)
		}
	}
	return out
}

func matchDS(keys []*wire.DNSKEY, records []*wire.DS) []*wire.DNSKEY {
	var out []*wire.DNSKEY
	for _, key := range keys {
		for _, ds := range records {
			candidate := key.ToDS(ds.DigestType)
			if candidate != nil && candidate.KeyTag == ds.KeyTag && candidate.Algorithm == ds.Algorithm && strings.EqualFold(candidate.Digest, ds.Digest) {
				out = append(out, key)
				break
			}
		}
	}
	return out
}

func dnssecQuery(ctx context.Context, name string, kind uint16, exchange dnssecExchange) (*wire.Msg, error) {
	return exchange(ctx, question(name, kind))
}

func question(name string, kind uint16) *wire.Msg {
	q := new(wire.Msg)
	q.SetQuestion(name, kind)
	q.CheckingDisabled = true
	q.SetEdns0(1232, true)
	return q
}
