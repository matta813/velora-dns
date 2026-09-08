# Operational REST API

Base path: `/api/v1`. Successful JSON responses contain `data`. Errors contain
`error: {code, message}`. This unauthenticated development API belongs on a trusted
management interface. One middleware boundary is reserved for future authentication.

| Method | Path | Behavior |
|---|---|---|
| GET | /health | HTTP process liveness |
| GET | /ready | 200 when SQLite and DNS are ready; otherwise 503 |
| GET | /api/v1/status | Listener readiness, uptime, version and implemented capabilities |
| GET | /api/v1/version | Build version, source commit and build timestamp |
| GET | /api/v1/stats | Lifetime queries and rolling 60-second QPS; cache hit ratio |
| GET | /api/v1/cache | Live entries, capacity, lifetime hits and misses |
| DELETE | /api/v1/cache | Clear cached answers; preserve lifetime counters |
| GET | /api/v1/config | Current config, excluding database path |
| GET | /api/v1/blocklists | Blocklist sources with domain counts and status |
| POST | /api/v1/blocklists | Add an HTTP(S) or local blocklist source |
| PUT | /api/v1/blocklists/{id} | Enable or disable a source |
| PUT | /api/v1/blocklists/{id}/content | Replace a local source's domains |
| POST | /api/v1/blocklists/{id}/update | Refresh a remote source, preserving prior rules on failure |
| DELETE | /api/v1/blocklists/{id} | Remove a source and its domains |
| GET | /metrics | Prometheus exposition |

```bash
curl http://127.0.0.1:8080/api/v1/status
curl -X DELETE -H 'Content-Type: application/json' http://127.0.0.1:8080/api/v1/cache
```

Bodies are limited to 1 MiB, headers to 16 KiB, concurrent HTTP requests to 32, with read,
write and idle timeouts. Cross-site browser requests and mismatched Origin are rejected.
HTTP Host must match the configured allowlist to reject DNS rebinding.
Cache mutation requires `application/json`. Reverse proxies should preserve the public
Host and Origin consistently; no permissive CORS is provided.

Zone and record CRUD are available; see the [zone API contract](zones.md).
Query history is available via [the query logging API](query-logging.md).
Blocklist sources support add, enable/disable, manual content and refresh. Remote
sources require public HTTP(S) URLs on ports 80/443 without credentials, fragments or
redirects; downloads are bounded to 8 MiB with SSRF protection. A failed refresh retains
the previous domains. Authentication is not implemented.
Unknown API paths return 404. There is no wildcard route returning fabricated data.
Config duration values serialize as nanoseconds. QPS is query count in the last 60
seconds divided by 60; this includes the initial partial minute.

## Metrics

- `dns_queries_total{type,source,rcode}`
- `dns_queries_blocked_total` (counts queries blocked by policy)
- `dns_cache_hits_total`
- `dns_cache_misses_total`
- `dns_upstream_requests_total{upstream}`
- `dns_upstream_errors_total{upstream}`
- `dns_query_duration_seconds` (histogram)
- `dns_cache_entries`
- `dns_tcp_connections`
- `dns_overload_total{reason}` where reason is one of the server-defined bounded values
  `client_rate`, `global_rate`, `concurrency`, or `tcp_connections`

Labels use only known query types, fixed pipeline sources, known response codes and
configured upstream addresses. No domain or client-IP labels. Sources currently include
`cache`, `upstream`, `refused`, `overload` and `blocked`. Counter vectors appear after
their first observation. Scrapes are limited
to five concurrent requests. Prometheus should use a private management endpoint.
