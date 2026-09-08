package dns

import (
	"fmt"
	"strings"

	wire "github.com/miekg/dns"
)

// validateAnswerChain prevents unrelated answer data and malformed alias graphs from
// entering the cache. Authority and additional data remain subject to normal DNS rules.
func validateAnswerChain(q, response *wire.Msg) error {
	if len(q.Question) != 1 {
		return fmt.Errorf("invalid question count")
	}
	current := strings.ToLower(q.Question[0].Name)
	seen := map[string]bool{current: true}
	aliases := 0
	terminal := false
	for _, rr := range response.Answer {
		if rr.Header().Rrtype == wire.TypeRRSIG {
			continue
		}
		owner := strings.ToLower(rr.Header().Name)
		if owner != current {
			return fmt.Errorf("unrelated answer record")
		}
		cname, ok := rr.(*wire.CNAME)
		if !ok {
			terminal = true
			continue
		}
		if terminal {
			return fmt.Errorf("CNAME follows terminal answer")
		}
		aliases++
		target := strings.ToLower(cname.Target)
		if aliases > 16 || seen[target] {
			return fmt.Errorf("invalid CNAME chain")
		}
		seen[target] = true
		current = target
		if q.Question[0].Qtype == wire.TypeCNAME {
			terminal = true
		}
	}
	return nil
}
