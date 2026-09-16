package dns

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	wire "github.com/miekg/dns"
)

func TestDoHGETAndPOST(t *testing.T) {
	handler := DoH(wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {
		m := new(wire.Msg)
		m.SetReply(q)
		_ = w.WriteMsg(m)
	}))
	query := new(wire.Msg)
	query.SetQuestion("example.test.", wire.TypeA)
	payload, err := query.Pack()
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/dns-query?dns="+base64.RawURLEncoding.EncodeToString(payload), nil),
		httptest.NewRequest(http.MethodPost, "/dns-query", bytes.NewReader(payload)),
	} {
		if request.Method == http.MethodPost {
			request.Header.Set("Content-Type", "application/dns-message; charset=binary")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		if w.Code != 200 || w.Header().Get("Content-Type") != dnsMessageMediaType {
			t.Fatalf("DoH: %d %s", w.Code, w.Body.String())
		}
		response := new(wire.Msg)
		if err = response.Unpack(w.Body.Bytes()); err != nil || !response.Response || response.Id != query.Id {
			t.Fatalf("response: %v %v", response, err)
		}
	}
}

func TestDoHUpstream(t *testing.T) {
	server := httptest.NewTLSServer(DoH(wire.HandlerFunc(answer)))
	t.Cleanup(server.Close)
	transport := server.Client().Transport.(*http.Transport)
	query := new(wire.Msg)
	query.SetQuestion("example.test.", wire.TypeA)
	response, used, err := (&Forwarder{Upstreams: []string{server.URL + "/dns-query"}, Timeout: time.Second, TLSConfig: transport.TLSClientConfig}).Resolve(t.Context(), query)
	if err != nil || len(response.Answer) != 1 || used != server.URL+"/dns-query" {
		t.Fatalf("DoH upstream: %v %s %v", response, used, err)
	}
}

func TestDoHRejectsInvalidRequests(t *testing.T) {
	handler := DoH(wire.HandlerFunc(func(w wire.ResponseWriter, q *wire.Msg) {}))
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/dns-query?dns=bad!", nil),
		httptest.NewRequest(http.MethodPost, "/dns-query", bytes.NewReader([]byte{1})),
		httptest.NewRequest(http.MethodPut, "/dns-query", nil),
	} {
		request.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		if w.Code < 400 {
			t.Fatalf("accepted invalid %s request", request.Method)
		}
	}
}
