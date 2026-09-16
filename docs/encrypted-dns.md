# Encrypted DNS

Velora supports DNS-over-TLS (DoT), DNS-over-HTTPS (DoH) and DNS-over-QUIC (DoQ).
All encrypted listeners reuse the normal ACL, rate limits, resolver, cache, filtering
and query-log policy.

## DNS-over-TLS and DNS-over-HTTPS

```yaml
dns:
  dot_listen: '127.0.0.1:853'
  doh_listen: '127.0.0.1:8443'
  tls_cert_file: /run/secrets/velora-cert.pem
  tls_key_file: /run/secrets/velora-key.pem
  upstreams: ['tls://1.1.1.1:853', 'https://1.1.1.1:443/dns-query']
```

The certificate and unencrypted private key must be readable by UID 10001. Mount
them read-only and restrict the key to that account. Velora requires TLS 1.2 or newer
and reloads a changed certificate/key pair on the next handshake. Invalid replacement
files reject new handshakes while the process remains running; deploy both files
atomically where possible.

DoH is served only at `/dns-query`. GET uses unpadded base64url in the `dns` query
parameter; POST requires `application/dns-message`. Requests and responses are bounded
to the DNS wire maximum. The endpoint deliberately does not enable CORS.

## DNS-over-QUIC

```yaml
dns:
  doq_listen: '0.0.0.0:853'
```

DNS-over-QUIC implements [RFC 9250](https://www.rfc-editor.org/rfc/rfc9250). Each DNS
query is carried over a QUIC stream, providing encryption and multiplexing without the
head-of-line blocking issues of TCP. DoQ uses the `doq` ALPN identifier and requires
TLS 1.3.

DoQ does not require separate TLS certificate files; the QUIC transport handles its own
TLS handshake. The listener accepts connections and processes DNS queries on individual
streams, using the same resolver pipeline as UDP/TCP listeners.

## Upstream configuration

Encrypted upstreams use `tls://IP:port` or `https://IP:port/dns-query`. QUIC upstreams
use `quic://IP:port`. All upstream URLs require IP literals to avoid an implicit
bootstrap-DNS dependency. The upstream certificate therefore needs a matching IP
subject alternative name.

## Smoke tests

```bash
# DNS-over-TLS
dig +tls @127.0.0.1 -p 853 example.org A

# DNS-over-HTTPS
curl --http2 --data-binary @query.bin -H 'Content-Type: application/dns-message' \
  https://127.0.0.1:8443/dns-query --output response.bin

# DNS-over-QUIC
dig +quic @127.0.0.1 -p 853 example.org A
```
