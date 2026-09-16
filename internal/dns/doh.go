package dns

import (
	"encoding/base64"
	"io"
	"mime"
	"net"
	"net/http"

	wire "github.com/miekg/dns"
)

const dnsMessageMediaType = "application/dns-message"

func DoH(handler wire.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dns-query" {
			http.NotFound(w, r)
			return
		}
		var payload []byte
		var err error
		switch r.Method {
		case http.MethodGet:
			payload, err = base64.RawURLEncoding.DecodeString(r.URL.Query().Get("dns"))
		case http.MethodPost:
			mediaType, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if parseErr != nil || mediaType != dnsMessageMediaType {
				http.Error(w, "use application/dns-message", http.StatusUnsupportedMediaType)
				return
			}
			payload, err = io.ReadAll(io.LimitReader(r.Body, 65536))
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err != nil || len(payload) == 0 || len(payload) > 65535 {
			http.Error(w, "invalid DNS message", http.StatusBadRequest)
			return
		}
		request := new(wire.Msg)
		if err = request.Unpack(payload); err != nil {
			http.Error(w, "invalid DNS message", http.StatusBadRequest)
			return
		}
		writer := &dohWriter{remote: addr(r.RemoteAddr)}
		handler.ServeDNS(writer, request)
		if writer.err != nil || len(writer.message) == 0 {
			http.Error(w, "DNS response unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", dnsMessageMediaType)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(writer.message)
	})
}

type dohWriter struct {
	message []byte
	err     error
	remote  net.Addr
}

func (w *dohWriter) LocalAddr() net.Addr  { return addr("127.0.0.1:443") }
func (w *dohWriter) RemoteAddr() net.Addr { return w.remote }
func (w *dohWriter) WriteMsg(message *wire.Msg) error {
	w.message, w.err = message.Pack()
	return w.err
}
func (w *dohWriter) Write(payload []byte) (int, error) {
	w.message = append([]byte(nil), payload...)
	return len(payload), nil
}
func (w *dohWriter) Close() error        { return nil }
func (w *dohWriter) TsigStatus() error   { return nil }
func (w *dohWriter) TsigTimersOnly(bool) {}
func (w *dohWriter) Hijack()             {}

func addr(value string) net.Addr {
	address, err := net.ResolveTCPAddr("tcp", value)
	if err != nil {
		return &net.TCPAddr{}
	}
	return address
}

var _ wire.ResponseWriter = (*dohWriter)(nil)
