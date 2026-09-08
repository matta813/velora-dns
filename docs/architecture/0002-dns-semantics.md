# ADR 0002: Conservative forwarding and cache semantics

Status: accepted.

The foundation accepts one IN question per recursive query. Supported types are A, AAAA,
CNAME, TXT, MX, NS and PTR. ANY, transfers and nonrecursive forwarding are refused.
Client ACL precedes the resolver. Upstreams are attempted in configured order, then retried
in whole rounds; transient SERVFAIL/REFUSED and transport errors fail over. NXDOMAIN is
returned as an authoritative upstream outcome and is not retried. Responses must match
the question. A truncated UDP response retries using TCP within the same attempt deadline.

Outbound EDNS advertises the actual 1232-byte receive capacity so large upstream answers
can signal truncation and use TCP. IPv4 and IPv6 sockets bind independently.
Client-facing UDP replies respect advertised size with a 1232-byte ceiling; ordinary
queries use 512 bytes. Truncation allows the client to retry TCP. TCP answers are not
artificially truncated. The total resolution deadline is five seconds. TCP reads/writes,
idle connections and queries per connection are bounded.

The cache key includes case-folded question, type, class, RD, CD and DO bits. Returned
messages are copied, query IDs/question case restored and TTLs aged. Positive TTL uses the
minimum across response records, capped at one day. RFC 2308 NXDOMAIN and NODATA responses
are cached only when they carry an SOA; their TTL is the minimum of SOA TTL and MINIMUM,
also capped at one day. Zero-TTL, truncated, large and client-option-dependent responses
bypass cache. Half-complete negative responses without an SOA are never cached.

EDNS version 0 is accepted with a 1232-byte server ceiling. Multiple/misplaced OPT records
and malformed or duplicate DNS Cookies are rejected. DNS Cookies bind a client cookie to
the source address with a process-local keyed HMAC; an invalid server cookie receives
BADCOOKIE and never reaches resolution. Arbitrary client EDNS options are accepted for
compatibility but make the request uncacheable and are not forwarded upstream. Only DO and
the server receive size are propagated, preventing ECS, Cookie and other client metadata
from leaking. Upstream EDNS options are removed before returning a response; the resolver
retains the response OPT only when the client used EDNS.

Forwarded CNAME answers must form an ordered, loop-free chain of at most 16 aliases and
cannot contain unrelated answer records. Malformed chains fail over instead of entering
the cache.

There is no local DNSSEC validation. AD is cleared even if an upstream sets it, avoiding
an unsupported validation assertion. Future validation, authoritative zones and blocking
must be independent resolver stages with explicit response provenance and cache invalidation.

SQLite schema migration is embedded under `internal/database/migrations`; there is only
one migration location. Domain repositories will define operations at the owning package
boundary when implemented. Avoid speculative interfaces without consumers.
