# Configuration

Precedence: built-in defaults → optional YAML file → explicit environment variables.
Missing files, unknown fields, multiple documents and invalid overrides are errors.
The server prints structured errors and exits nonzero before reporting readiness.

| YAML key | Environment variable | Default |
|---|---|---|
| dns.listen | VELORA_DNS_LISTEN | 127.0.0.1:5353 |
| dns.upstreams | VELORA_DNS_UPSTREAMS | 1.1.1.1:53,9.9.9.9:53 |
| dns.allowed_clients | VELORA_DNS_ALLOWED_CLIENTS | 127.0.0.0/8,::1/128 |
| dns.timeout | VELORA_DNS_TIMEOUT | 2s |
| dns.retries | VELORA_DNS_RETRIES | 1 |
| dns.max_concurrent | VELORA_DNS_MAX_CONCURRENT | 256 |
| dns.rate_per_second | VELORA_DNS_RATE_PER_SECOND | 200 |
| dns.rate_burst | VELORA_DNS_RATE_BURST | 400 |
| dns.global_rate_per_second | VELORA_DNS_GLOBAL_RATE_PER_SECOND | 5000 |
| dns.global_rate_burst | VELORA_DNS_GLOBAL_RATE_BURST | 10000 |
| dns.rate_clients | VELORA_DNS_RATE_CLIENTS | 4096 |
| dns.max_tcp_connections | VELORA_DNS_MAX_TCP_CONNECTIONS | 256 |
| cache.max_entries | VELORA_CACHE_MAX_ENTRIES | 10000 |
| filtering.block_mode | VELORA_FILTERING_BLOCK_MODE | (NXDOMAIN) |
| filtering.blocklist | VELORA_FILTERING_BLOCKLIST | (empty) |
| filtering.allowlist | VELORA_FILTERING_ALLOWLIST | (empty) |
| http.listen | VELORA_HTTP_LISTEN | 127.0.0.1:8080 |
| http.allowed_hosts | VELORA_HTTP_ALLOWED_HOSTS | localhost,127.0.0.1,::1 |
| http.web_dir | VELORA_WEB_DIR | web/dist |
| database_path | VELORA_DATABASE_PATH | data/velora.db |
| log_level | VELORA_LOG_LEVEL | info |

List overrides are comma-separated. Duration overrides use Go durations such as `500ms`
or `2s`. Timeout is 10ms–10s; retries are 0–3 additional rounds across all upstreams.
The overall request deadline is five seconds, so long retry configurations may not
complete every configured attempt. UDP-to-TCP fallback shares an attempt's timeout.
Upstreams and listeners require IP literals and explicit ports, avoiding bootstrap DNS
and accidental hostname dependency. DNS supports up to eight listeners and eight upstreams.

Cache size zero disables caching; maximum is 1,000,000 entries. Concurrent DNS work is
1–10,000 requests. Choose realistic limits for your memory budget; Compose defaults to
256 MiB. Large DNS responses over 16 KiB are not cached. Expiry sweep runs each second;
expired entries are also rejected immediately on lookup.

DNS rate limits use token buckets shared by UDP and TCP. Each source address receives
`rate_per_second` tokens per second up to `rate_burst`; all clients also share the global
rate and burst. Rate-limited requests receive REFUSED without reaching the resolver.
The per-client table retains at most `rate_clients` addresses and replaces entries in
constant time; rotating-source attacks remain bounded by the global bucket. Bursts must
cover at least one second, and global limits must be no smaller than per-client limits.
All six values accept 1–1,000,000. `max_tcp_connections` bounds accepted connections
across each listener; excess connections are closed immediately. Resolver work remains
separately bounded by `max_concurrent`, and TCP keeps its five-second idle timeout and
100-query-per-connection cap.

`VELORA_DNS_PORT` and `VELORA_HTTP_PORT` are Compose host-port substitutions, not Go
server settings. Compose deliberately overrides listener and data paths for the container.
The default config file is a development example; it is not automatically loaded.

HTTP Host values must match `http.allowed_hosts` (ports are ignored). Add the exact
management hostname or IP for a reverse proxy or LAN interface. Wildcards are rejected
to prevent DNS rebinding from bypassing the loopback management boundary.

`filtering.block_mode` selects the response for blocked names: `NXDOMAIN` (default) or
`ZERO` (an all-zero A/AAAA answer). Wildcard blocklist entries match the domain and its
subdomains; an exact allowlist entry always wins. External blocklist sources are managed
through the API and dashboard, not this configuration.
