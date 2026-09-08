package dns

import (
	"net/netip"
	"testing"

	wire "github.com/miekg/dns"
)

func FuzzProtocolPolicies(f *testing.F) {
	q := new(wire.Msg)
	q.SetQuestion("example.test.", wire.TypeA)
	packed, err := q.Pack()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(packed)
	f.Add([]byte{0, 1, 2, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		var msg wire.Msg
		if msg.Unpack(data) != nil {
			return
		}
		_, _ = clientEDNS(&msg, netip.MustParseAddr("192.0.2.1"), []byte("fuzz secret"))
		if len(msg.Question) == 1 {
			_ = validateAnswerChain(&msg, &msg)
		}
	})
}
