# Operational REST API

The [OpenAPI 3.1 specification](openapi.json) lists the current management
routes, response envelopes, request models, authentication and roles. When
changing an API route or model, update `scripts/generate-openapi.py`, regenerate
with `python3 scripts/generate-openapi.py`, and run `make openapi-check`. CI
checks the generated document against registered Go routes.

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
| POST | /api/v1/diagnostics/explain | Side-effect-free "why this answer": `{name, type?, client?}` returns the winning stage and every stage considered, with rule IDs and cache TTL ([details](#answer-explanation)); viewers and read tokens allowed |
| POST | /api/v1/backup/create | Admin-only encrypted SQLite configuration and state bundle download |
| POST | /api/v1/backup/inspect | Admin-only multipart upload (`passphrase` first, then `bundle`) that validates a backup without changing anything and returns metadata, a content summary, warnings and a 30-minute `token` |
| POST | /api/v1/backup/restore | Admin-only: `{token, confirm: true}` schedules the inspected backup and restarts Velora to apply it (`202`, state `restarting`) |
| GET | /api/v1/backup/restore | Admin-only result of the last online restore: `pending`, `applied`, `completed`, `rolled_back` or `failed` ([details](backup.md#restore-from-the-web-interface)) |
| GET | /api/v1/audit | Admin-only audit events with actor, action, result and cursor filters |
| GET | /api/v1/events | Recent system events and unread count, filtered by role |
| POST | /api/v1/events/{id}/read | Mark an accessible event as read for the current user |
| GET | /api/v1/version | Build version, source commit and build timestamp |
| GET | /api/v1/update/check | Installed/latest version, channel, release details and availability |
| GET | /api/v1/update/status | Current updater phase, errors and rollback/readiness result |
| GET | /api/v1/update/history | Persistent update attempts and final results |
| POST | /api/v1/update/request | Start the release selected by the configured updater agent |
| GET | /api/v1/stats | Persisted query counters, rolling 60-second QPS and cache hit ratio |
| POST | /api/v1/stats/reset | Reset persisted query, cache and rate-limit counters (admin only) |
| GET | /api/v1/cluster/overview | This node's cluster role, zone revision and, on a primary, replicas with sync status |
| POST | /api/v1/cluster/create | Admin: make this node a primary (`name`, `advertised_url`, optional `allow_insecure`, `skip_check`) |
| POST | /api/v1/cluster/join-tokens | Admin, primary: one-time join token valid for 30 minutes |
| POST | /api/v1/cluster/connect | Admin: join a primary as a replica (`primary_url`, `token`, `name`, `allow_insecure`) |
| POST | /api/v1/cluster/sync | Admin, replica: sync now |
| DELETE | /api/v1/cluster/members/{id} | Admin, primary: remove a replica |
| POST | /api/v1/cluster/leave | Admin, replica: leave the cluster |
| POST | /api/v1/cluster/dissolve | Admin, primary: revoke all replicas and become standalone |
| GET | /api/v1/search?q= | Command palette search (2–100 characters) over zones, records, clients, rewrites, forwarding rules and blocklists; at most 5 results per kind and a bounded record scan |
| GET | /api/v1/analytics?range= | Bucketed query history for `1h`, `24h`, `7d` or `30d` from the query log: totals, time series, query types, response codes, sources, upstream usage and top blocked domains ([details](query-logging.md#analytics)) |
| GET | /api/v1/settings/rate-limit/status | Enabled state, persisted rejection count and last rejection time; no client addresses |
| GET | /api/v1/upstreams/health | Passive upstream health, recent latency and failure count for configured resolvers |
| GET | /api/v1/cache | Live entries, capacity, persisted hits and misses |
| DELETE | /api/v1/cache | Clear cached answers; preserve lifetime counters |
| GET | /api/v1/cache/entries | Page through cached answers (`limit` 1–200, `offset`); filter with `domain` (case-insensitive substring) and `type` (for example `AAAA`) |
| POST | /api/v1/cache/invalidate | Remove cached answers for `name`; optional `type` limits it to one record type and `include_subdomains` also removes names below it |
| GET | /api/v1/config | Current config, excluding database path and secrets |
| PUT | /api/v1/config | Validate and apply supported live settings, then save atomically; `400 invalid_config` and `409 config_requires_restart` list the affected `fields` |
| POST | /api/v1/config/validate | Dry run: validate a candidate (same body as PUT) and list field errors and restart-only changes without applying or saving anything |
| GET | /api/v1/clients | Named clients with 24-hour activity, plus busy unnamed addresses seen recently |
| POST | /api/v1/clients | Create a client: `name`, `addresses` (1–16 IPs or CIDR networks), optional `group`, `description`, `enabled` |
| PUT | /api/v1/clients/{id} | Replace a client |
| DELETE | /api/v1/clients/{id} | Remove a client and its filtering policy |
| GET | /api/v1/webhooks | Webhooks with delivery status (admin only; tokens are never returned) |
| POST | /api/v1/webhooks | Create a webhook: `name`, `url`, optional `events`, `min_severity`, `allow_private`, `enabled`, write-only `token` (admin only) |
| PUT | /api/v1/webhooks/{id} | Replace a webhook; omit `token` to keep it, `""` removes it (admin only) |
| DELETE | /api/v1/webhooks/{id} | Remove a webhook (admin only) |
| POST | /api/v1/webhooks/{id}/test | Send a `webhook.test` event now and return the result (admin only) |
| GET | /api/v1/webhooks/event-types | Event types a webhook can subscribe to (admin only) |
| GET | /api/v1/policies | Per-client filtering policies |
| POST | /api/v1/policies | Create a policy: `client_id`, `mode` (`default`, `disabled` or `custom`), and for custom mode `blocklists` (source IDs), `allow` and `block` domains; optional `enabled` |
| PUT | /api/v1/policies/{id} | Replace a policy |
| DELETE | /api/v1/policies/{id} | Remove a policy; the client returns to global filtering |
| GET | /api/v1/policies/effective?ip= | The client and policy that apply to an address |
| GET | /api/v1/rewrites | Local DNS rewrites with precedence hints (`blocked_by`, `overrides_zone`) |
| POST | /api/v1/rewrites | Create a rewrite: `name` (host or `*.parent`), `type` (A, AAAA, CNAME), `value`, optional `enabled`, `description` |
| PUT | /api/v1/rewrites/{id} | Replace a rewrite |
| DELETE | /api/v1/rewrites/{id} | Remove a rewrite |
| GET | /api/v1/forwarding | Conditional forwarding rules with live upstream health |
| POST | /api/v1/forwarding | Create a rule: `domain`, `upstreams` (1–4 `IP` or `IP:port`), optional `enabled`, `description` |
| PUT | /api/v1/forwarding/{id} | Replace a rule |
| DELETE | /api/v1/forwarding/{id} | Remove a rule |
| POST | /api/v1/forwarding/{id}/test | Send a diagnostic query (`name` within the rule, `type`, default A) through the rule's upstreams only |
| GET | /api/v1/blocklists | Blocklist sources with domain counts and status |
| POST | /api/v1/blocklists | Add an HTTP(S) or local blocklist source; optional `update_interval` schedules automatic refreshes |
| PUT | /api/v1/blocklists/{id} | Enable or disable a source (`enabled`) and/or change its refresh schedule (`update_interval`) |
| PUT | /api/v1/blocklists/{id}/content | Replace a local source's domains |
| POST | /api/v1/blocklists/{id}/update | Refresh a remote source, preserving prior rules on failure |
| DELETE | /api/v1/blocklists/{id} | Remove a source and its domains |
| GET | /metrics | Prometheus exposition |

Statistics are checkpointed to the management database every five seconds and on
clean shutdown. A crash can lose up to one checkpoint interval of counts. The
admin-only reset clears cumulative query, blocked, cache hit/miss, upstream
request/error, and rate-limit rejection counters. It keeps cached answers,
query logs, zones, configuration, uptime, and process-only latency histograms.
The rolling QPS window starts again at zero. Cache flushing uses its separate
`DELETE /api/v1/cache` action.

Configuration updates are applied to the running services and saved only after
validation and readiness checks. Failed apply or save attempts restore the
previous live configuration. Listener addresses, upstreams, TLS settings and
other fields without a live apply path return `config_requires_restart` with
field names; the candidate is neither activated nor saved. Change those fields
in the configuration file and restart the service.

The flow for `PUT /api/v1/config` is: merge the request onto the active
configuration → validate every field → apply live → check DNS and database
readiness → save the YAML file atomically (temporary file, then rename) →
make it the active configuration. Success is reported only after all steps. Any
failure after apply restores the previous settings
and reports `config_apply_failed`, `config_not_ready` or `config_save_failed`;
if the rollback itself fails the response is `config_rollback_failed` and a
critical system event is raised. Changes are serialized, so concurrent saves
never interleave.

Validation errors name each field with its configuration path, for example:

```json
{"error": {"code": "invalid_config", "message": "dns.allowed_clients[1]: invalid client network \"lan\"; …",
  "fields": [{"field": "dns.allowed_clients[1]", "message": "invalid client network \"lan\"; use CIDR notation such as 192.168.1.0/24"}]}}
```

Validation covers listen and upstream addresses and ports, client networks,
allowed hosts, filtering domains, cache, rate-limit and query-log bounds, TLS,
DNSSEC anchors, database settings and paths (no `..` segments, URLs or control
characters), and listeners that would bind the same port and protocol — for
example the web interface on TCP/53 next to a wildcard DNS listener. Actual
port availability is checked when a restart-only listener change is started
with the new file; the running service is never torn down by an API request.

System events currently include upstream outage/recovery and configuration
rollback outcomes. Repeated events with the same key are combined for ten
minutes and become unread again. Events are retained for 90 days, capped at
1,000 rows. Configuration rollback events are visible only to admins; upstream
availability events are visible to all authenticated users. Read state is per
user and requires CSRF for session requests.

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

## Answer explanation

`POST /api/v1/diagnostics/explain` runs a dry run of the resolver
([ADR 0006](architecture/0006-answer-explanation.md)):

```json
{"name": "nas.home", "type": "A", "client": "192.168.1.40"}
```

`name` (required) is at most 253 characters of letters, digits, hyphens,
underscores and dots; `type` defaults to `A` and must be `A`, `AAAA`, `CNAME`,
`TXT`, `MX`, `NS`, `PTR` or `SOA`; `client` must be an IPv4 or IPv6 address and
selects that client's filtering policy. Invalid input returns `400`
(`invalid_name`, `invalid_type`, `invalid_client`, `invalid_json`), a wrong content
type `415`.

The response lists `source` (what a real query would report: `local`, `cache`,
`blocked`, `upstream`), the `winner` step, the `rcode` and `answers` for local and
cached results, and every `step` in resolver order: `filter`, `rewrite`, `zone`,
`cache`, then `forwarding` or `upstream`. Steps carry rule identifiers (rewrite ID,
zone, blocklist source, policy, forwarding rule) and, for cache hits, the
`remaining_ttl`. A step with `depth` above 0 belongs to a CNAME target.

The request never contacts an upstream: a name that would be forwarded is reported
as `would_forward` without an answer. It writes no query log entry and changes no
cache entry, statistic or metric, and it is not audited. Viewers and read-scoped
tokens may call it even though it is a POST; browser sessions still send the CSRF
header.
