# ADR 0006: Answer explanation

## Status

Accepted and implemented.

## Context

When a name resolves to something unexpected, operators need to know which stage
answered: a rewrite, a local zone, a blocklist or client policy, the cache, a
conditional forwarding rule or the global upstreams. Reproducing the query with
`dig` is not enough: it hits the real cache and query log, cannot show the
client-specific policy, and says nothing about the rule behind the answer.

## Decision

`POST /api/v1/diagnostics/explain` runs a dry run of the resolver and reports the
stages it considered, the winner and the rule identifiers.

### Real precedence

Read from `Resolver.resolve` (`internal/dns/resolver.go`), `rewrites.Local.Lookup`
(`internal/rewrites/rewrites.go`) and `forwarding.Router.Resolve`
(`internal/forwarding/forwarding.go`), the first applicable stage wins:

1. **Filter** (`blockedName`): the querying client's policy if the address belongs
   to a client with an enabled non-default policy, otherwise the global
   configuration rules and enabled blocklists. A blocked name answers immediately
   (`blocked`). Allow rules override blocks.
2. **Rewrites**, then **local zones** (`rewrites.Local.Lookup`). A zone that owns
   the name answers authoritatively, including NXDOMAIN, and never falls through.
   The local answer is checked against the filter again (`blockedAnswer`), and a
   local CNAME is followed through the same stages with the target name.
3. **Cache**, only for recursive queries. A hit is checked against the filter
   again, so a policy never leaks through the cache.
4. **Upstream**: the most specific enabled conditional forwarding rule
   (`Router.Resolve`), otherwise the global upstreams. The upstream answer is
   checked against the filter before it is cached.

Client access control, rate limiting, EDNS checks and the concurrency limit run
earlier in `Handler.ServeDNS` and are not part of the explanation. This matches the
precedence lists in the user documentation; the cache is consulted before the
forwarding rule is chosen.

### One code path

`Resolver.resolve` takes an optional trace. With a trace it only peeks at the cache
(`cache.Peek`: no hit/miss counter, no recency change, no expiry removal), skips the
upstream call and the cache insert, and records each stage. Real queries pass no
trace, so the decision logic cannot drift between the two. Rules are identified by
optional interfaces (`LocalExplainer`, `RouteExplainer`, `FilterExplainer`) that the
rewrite, forwarding and policy services implement on top of the same lookups.

### API

Request body (JSON, `Content-Type: application/json`):

```json
{"name": "nas.home", "type": "A", "client": "192.168.1.40"}
```

- `name` is required, a valid DNS name of at most 253 characters (a trailing dot is allowed).
- `type` defaults to `A` and must be one the resolver serves: `A`, `AAAA`, `CNAME`,
  `TXT`, `MX`, `NS`, `PTR`, `SOA`.
- `client` is optional and must parse as an IPv4 or IPv6 address.

The response lists `source` (what a real query would report), `winner`, the answer
that would be returned for local and cached results, and every considered `step`
with its rule identifiers (rewrite ID, zone, blocklist source, policy, forwarding
rule) and, for cache hits, the remaining TTL.

### Limits and safety

- The explanation never contacts an upstream. When no local stage or cache entry
  answers it reports `would_forward` with the rule or the global upstreams, and no
  answer. A live upstream probe is out of scope (the forwarding rule **Test** action exists).
- It writes no query log entry, changes no cache entry or statistics, and
  increments no metric. The audit log is skipped because the request changes nothing.
- Request bodies keep the global 1 MiB limit; all fields are bounded as above.
- Viewers and read-scoped API tokens may call it. The server middleware blocks
  unsafe methods for viewers, with a narrow exception for marking events read; the
  explain route is a second, equally narrow exception. CSRF protection still applies
  to browser sessions. Unauthenticated requests get `401`.

## Consequences

The explain result is only as current as the in-memory snapshots, like real
queries. Because the query is simulated with recursion desired, a client that sends
non-recursive queries may see `REFUSED` for names that need the cache or upstream.
