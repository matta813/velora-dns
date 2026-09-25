# Operational REST API

Base path: `/api/v1`. Successful JSON responses contain `data`. Errors contain
`error: {code, message}`. Health and readiness are public; management endpoints and
metrics require an authenticated session or scoped API token.

## Endpoints

| Method | Path | Behavior |
|---|---|---|
| GET | /health | HTTP process liveness |
| GET | /ready | 200 when SQLite/PostgreSQL and DNS are ready; otherwise 503 |
| POST | /api/v1/auth/login | Create a 12-hour management session |
| POST | /api/v1/auth/logout | Revoke the current session |
| GET | /api/v1/auth/me | Current role and per-session CSRF token |
| GET/POST | /api/v1/users | List/create users (admin only) |
| POST | /api/v1/tokens | Create a scoped token; secret returned once |
| DELETE | /api/v1/tokens/{id} | Revoke a token owned by the current user |
| GET | /api/v1/status | Listener readiness, uptime, version and implemented capabilities |
| GET | /api/v1/diagnostics | Sanitized system health and support report |
| GET | /api/v1/audit | Admin-only audit events with actor, action, result and cursor filters |
| GET | /api/v1/version | Build version, source commit and build timestamp |
| GET | /api/v1/update/check | Installed/latest version, channel, release details and availability |
| GET | /api/v1/update/status | Current updater phase, errors and rollback/readiness result |
| GET | /api/v1/update/history | Persistent update attempts and final results |
| POST | /api/v1/update/request | Start the release selected by the configured updater agent |
| GET | /api/v1/stats | Lifetime queries and rolling 60-second QPS; cache hit ratio |
| GET | /api/v1/settings/rate-limit/status | Enabled state, process-lifetime rejection count and last rejection time; no client addresses |
| GET | /api/v1/upstreams/health | Passive upstream health, recent latency and failure count for configured resolvers |
| GET | /api/v1/cache | Live entries, capacity, lifetime hits and misses |
| DELETE | /api/v1/cache | Clear cached answers; preserve lifetime counters |
| GET | /api/v1/config | Current config, excluding database path and secrets |
| GET | /api/v1/blocklists | Blocklist sources with domain counts and status |
| POST | /api/v1/blocklists | Add an HTTP(S) or local blocklist source |
| PUT | /api/v1/blocklists/{id} | Enable or disable a source |
| PUT | /api/v1/blocklists/{id}/content | Replace a local source's domains |
| POST | /api/v1/blocklists/{id}/update | Refresh a remote source, preserving prior rules on failure |
| DELETE | /api/v1/blocklists/{id} | Remove a source and its domains |
| GET | /metrics | Prometheus exposition |

## TSIG key management

| Method | Path | Behavior |
|---|---|---|
| GET | /api/v1/tsig-keys | List all TSIG keys (names and algorithms, secrets never returned) |
| POST | /api/v1/tsig-keys | Create a TSIG key (hmac-sha256, hmac-sha1, or hmac-sha512) |
| DELETE | /api/v1/tsig-keys/{name} | Delete a TSIG key |

## Secondary zone management

| Method | Path | Behavior |
|---|---|---|
| POST | /api/v1/zones/secondary | Create a secondary zone with primary address and transfer settings |
| GET | /api/v1/zones/{id}/transfer-status | Get transfer status and last serial for a secondary zone |

## Zone and record management

See the [zone API contract](zones.md) for full CRUD endpoints.

## Authentication

```bash
# Session-based login
curl -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"your-password"}'

# Bearer token
curl -H 'Authorization: Bearer velora_<secret>' http://127.0.0.1:8080/api/v1/status
```

API tokens use `Authorization: Bearer velora_<secret>`. Valid scopes are `read`,
`write` and `admin`; expiration is mandatory and limited to one year. Only a SHA-256
token digest is stored, and the secret is returned only by the creation response.
Cookie-authenticated mutations require `X-CSRF-Token`; bearer requests do not.

The admin-only audit view supports `actor`, `action`, `result`, `limit` (1–200)
and `before` (event ID) filters. Administrative requests are recorded before
dispatch and updated with their HTTP result afterward; incomplete requests
remain marked `pending`. Events retain the actor's role at the time of the
request and are removed after 90 days. Only the method and route path are
stored, never request bodies, passwords, session tokens or CSRF tokens.

Bodies are limited to 1 MiB, headers to 16 KiB, concurrent HTTP requests to 32, with read,
write and idle timeouts. Cross-site browser requests and mismatched Origin are rejected.
HTTP Host must match the configured allowlist to reject DNS rebinding.
Cache mutation requires `application/json`. Reverse proxies should preserve the public
Host and Origin consistently; no permissive CORS is provided.

Update reads require an authenticated user. Starting an update requires a writable
admin/operator session or token and the normal CSRF protection. The management process
forwards these requests over a `0660`, `root:velora` Unix socket; it never accepts a
download URL or filesystem path from the browser.

## Metrics

- `dns_queries_total{type,source,rcode}`
- `dns_queries_blocked_total` (counts queries blocked by policy)
- `dns_cache_hits_total`
- `dns_cache_misses_total`
- `dns_upstream_requests_total{upstream}`
- `dns_upstream_errors_total{upstream}`
- `dns_query_duration_seconds` (histogram)
- `dns_cache_entries`

Labels use only known query types, fixed pipeline sources, known response codes and
configured upstream addresses. No domain or client-IP labels. Sources currently include
`cache`, `upstream`, `refused`, `overload` and `blocked`. Counter vectors appear after
their first observation. Scrapes are limited
to five concurrent requests. Prometheus should use a private management endpoint.

For a Prometheus instance on the same host as Velora, create a Velora API token
with the `read` scope and place only the token value in a file readable by
Prometheus, for example `/etc/prometheus/velora-token`. A scrape job can then
use the existing authenticated management listener:

```yaml
scrape_configs:
  - job_name: velora-dns
    metrics_path: /metrics
    scrape_interval: 15s
    static_configs:
      - targets: ["127.0.0.1:8080"]
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/velora-token
```

If Prometheus runs on another host, configure a private, protected route to
the management listener and include its hostname in `http.allowed_hosts`.
Keep the token file out of version control. The scrape job uses Prometheus's
[`authorization.credentials_file` setting](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#http_config).

## Upstream health

Upstream health is based on actual query outcomes. A configured resolver starts
as `unknown` until its first attempt. One or two consecutive failed attempts
mark it `degraded`; three mark it `unavailable`. The forwarder skips unavailable
resolvers for 30 seconds, then allows one recovery attempt. A successful reply
restores `healthy` state and records the observed attempt latency. The existing
ordered failover continues to use another configured resolver while one is
cooling down. No synthetic DNS traffic is generated. State is process-local and
resets on restart.

## Diagnostics report

The authenticated `GET /api/v1/diagnostics` endpoint is the source for the Web UI
health page and its JSON download. It reports DNS listener, storage, configuration,
cache, query logging and updater state when the updater is configured. The report
contains the running version, OS, architecture, uptime and configured upstream
count. It excludes passwords, tokens, configuration values, query logs, client
addresses and domain names. A missing updater is shown as degraded; a failed DNS
listener or management database is shown as failed. All authenticated roles may
read the sanitized report.
