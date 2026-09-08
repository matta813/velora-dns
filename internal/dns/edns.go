package dns

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"

	wire "github.com/miekg/dns"
)

const serverCookieBytes = 16

// clientEDNS validates the EDNS structure and returns a cookie suitable for the
// response. Unknown options are accepted but never forwarded or shared in cache.
func clientEDNS(q *wire.Msg, client netip.Addr, secret []byte) (*wire.EDNS0_COOKIE, int) {
	var opt *wire.OPT
	for _, rr := range q.Extra {
		candidate, ok := rr.(*wire.OPT)
		if !ok {
			continue
		}
		if opt != nil || candidate.Hdr.Name != "." {
			return nil, wire.RcodeFormatError
		}
		opt = candidate
	}
	if opt == nil {
		return nil, wire.RcodeSuccess
	}
	if opt.Version() != 0 {
		return nil, wire.RcodeBadVers
	}
	var cookie []byte
	for _, option := range opt.Option {
		value, ok := option.(*wire.EDNS0_COOKIE)
		if !ok {
			continue
		}
		if cookie != nil {
			return nil, wire.RcodeFormatError
		}
		var err error
		cookie, err = hex.DecodeString(value.Cookie)
		if err != nil || (len(cookie) != 8 && (len(cookie) < 16 || len(cookie) > 40)) {
			return nil, wire.RcodeFormatError
		}
	}
	if cookie == nil {
		return nil, wire.RcodeSuccess
	}
	expected := serverCookie(secret, client, cookie[:8])
	response := &wire.EDNS0_COOKIE{Code: wire.EDNS0COOKIE, Cookie: hex.EncodeToString(append(append([]byte{}, cookie[:8]...), expected...))}
	if len(cookie) > 8 && !hmac.Equal(cookie[8:], expected) {
		return response, wire.RcodeBadCookie
	}
	return response, wire.RcodeSuccess
}

func serverCookie(secret []byte, client netip.Addr, cookie []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(client.Unmap().AsSlice())
	_, _ = mac.Write(cookie)
	return mac.Sum(nil)[:serverCookieBytes]
}

func addResponseCookie(m *wire.Msg, q *wire.Msg, cookie *wire.EDNS0_COOKIE) {
	if cookie == nil {
		return
	}
	opt := m.IsEdns0()
	if opt == nil {
		size := uint16(1232)
		if requestOPT := q.IsEdns0(); requestOPT != nil {
			size = requestOPT.UDPSize()
		}
		m.SetEdns0(size, false)
		opt = m.IsEdns0()
	}
	options := opt.Option[:0]
	for _, option := range opt.Option {
		if option.Option() != wire.EDNS0COOKIE {
			options = append(options, option)
		}
	}
	opt.Option = append(options, cookie)
}
