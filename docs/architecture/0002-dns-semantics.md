# ADR 0002: Conservative forwarding and cache semantics

Status: accepted.

The foundation accepts one IN question per recursive query. Supported types are A, AAAA,
CNAME, TXT, MX, NS and PTR. ANY, transfers and nonrecursive forwarding are refused.
Client ACL precedes the resolver. Upstreams are attempted in configured order, then retried
in whole rounds; transient SERVFAIL/REFUSED and transport errors fail over. NXDOMAIN is
returned as an authoritative upstream outcome and is not retried. Responses must match
the question. A truncated UDP response retries using TCP within the same attempt deadline.

Client-facing UDP replies respect advertised size with a 1232-byte ceiling; ordinary
queries use 512 bytes. Truncation allows the client to retry TCP. TCP answers are not
artificially truncated. The total resolution deadline is five seconds. TCP reads/writes,
idle connections and queries per connection are bounded.

The cache key includes case-folded question, type, class, RD, CD and DO bits. Returned
messages are copied, query IDs/question case restored and TTLs aged. TTL uses the minimum
across response records, capped at one day for retention. Zero-TTL, negative, truncated,
large and EDNS-option-dependent responses bypass cache. Negative caching awaits RFC 2308
SOA handling. Domain/client logging is absent in the foundation. Lifecycle logs cannot
silently grow into a query database.

There is no local DNSSEC validation. AD is cleared even if an upstream sets it, avoiding
an unsupported validation assertion. Future validation, authoritative zones and blocking
must be independent resolver stages with explicit response provenance and cache invalidation.

SQLite schema migration is embedded under `internal/database/migrations`; there is only
one migration location. Domain repositories will define operations at the owning package
boundary when implemented. Avoid speculative interfaces without consumers.
