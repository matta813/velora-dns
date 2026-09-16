# Production assessment

Assessed on 2026-09-16 against the current development baseline. This is an
operational readiness assessment, not a production approval: Velora DNS must not
be advertised as production-ready until every blocking item below is resolved and
the stated drills are performed in the target environment.

## Decision

| Area | Status | Evidence and action |
|---|---|---|
| Management access | Blocked | Issue #19 is open: the HTTP API has no users, sessions or roles. Keep it loopback-only and use an authenticated administrative tunnel. |
| Resolver transport | Conditional | UDP/TCP DNS works; DoT and DoH remain open (#16, #17). Use a trusted private network and firewall until encrypted transports are delivered. |
| DNS correctness | Conditional | Local integration tests cover supported record types, TCP fallback, EDNS, cookies and cache semantics. DNSSEC validation is still open (#18), so do not claim validated answers. |
| Abuse resistance | Conditional | Global/client query limits, concurrent-work and TCP-connection caps are configured. Size them with the procedure below and monitor rejection metrics. |
| Data durability | Conditional | SQLite persistence, migrations and WAL checkpointing are tested. Operators must complete a backup/restore drill before use. |
| Availability | Blocked | The service is a single-node deployment. HA, node membership and replication are tracked by #28–#32. |

## Security review checklist

Before deployment, record the reviewer, date and result for each control:

- Run `go test ./...`, `go vet ./...`, frontend tests/lint/typecheck and the Compose
  configuration test from `docs/validation.md` on the release candidate.
- Ensure management HTTP binds only to loopback; permit remote access only through
  an authenticated tunnel or a separately authenticated reverse proxy. Never expose
  the current unauthenticated API directly to a LAN or the Internet.
- Restrict `dns.allowed_clients` to actual internal CIDRs. Do not operate this
  recursive resolver publicly.
- Set global and per-client QPS, burst, concurrent-work and TCP-connection limits
  explicitly. Alert on `dns_overload_rejections_total`; its labels are bounded and
  intentionally contain no client identity.
- Use private, permission-restricted persistent storage. Query history can contain
  client addresses and queried names; set retention to the minimum operational need.
- Verify the image digest and SBOM/dependency scan results in CI. Do not enable
  release publication while `RELEASE_ENABLED` remains disabled.

## DNS compliance regression suite

The automated suite is the repeatable protocol evidence, not a claim of universal
DNS interoperability. Run it before every candidate:

```bash
go test ./internal/dns ./tests
```

It covers UDP and TCP listener behavior, supported IN record classes, local-zone
SOA/NXDOMAIN/NODATA behavior, upstream failover/retries, response/question and
CNAME-chain validation, EDNS size and privacy policy, DNS Cookies, and cache TTL
semantics. Track unsupported standards as explicit issues: DNSSEC (#18), DoT (#16),
DoH (#17), DoQ (#23), AXFR/IXFR (#25) and TSIG (#26).

## Sustained-load procedure and resource sizing

Benchmark only against a private test network and local upstream fixture; never load
test public resolvers. Capture the exact config, hardware, Go version, duration,
query mix, packet loss and results alongside the deployment record.

1. Start with `global_qps` at 60% of the measured sustainable QPS and `client_qps`
   at the largest justified client share; make `rate_limit_burst` no larger than one
   second of that client allowance.
2. Hold mixed UDP/TCP traffic for at least 30 minutes at 50%, 75% and 100% of the
   proposed global limit. Include cache hits, cache misses, blocked names and large
   answers that trigger TCP fallback.
3. Pass only if DNS latency remains within the deployment SLO, memory reaches a
   stable plateau, file descriptors remain below 70% of the process limit, and no
   unexpected `dns_overload_rejections_total` or query-log drops occur.
4. Repeat with intentionally abusive UDP/TCP traffic. Expected behavior is bounded
   memory and connections plus increasing rejection counters, not unbounded latency.

The initial container limit of 256 MiB is a development baseline, not a sizing
recommendation. Reserve headroom for SQLite, query logging, cache entries, TCP
connections and the operating system; choose final limits from observed peak usage.

## Upgrade, backup and restore drill

Perform this drill on a disposable copy of the production data volume before an
upgrade, then retain the command transcript and a checksum of the backup.

1. Stop the container cleanly and copy the complete named volume, including the
   SQLite database and WAL files, using the platform's volume-backup mechanism.
2. Start the new image with a copy of that volume. Confirm migrations complete,
   readiness is healthy, zones/blocklists/query-log policy are present, and DNS
   answers match pre-upgrade probes.
3. Stop it, replace the test volume with the backup, and start the prior known-good
   image. Confirm the same probes and management reads succeed.
4. Test a deliberately interrupted startup/migration against another disposable
   copy. Preserve the original volume; never downgrade a production volume without
   a verified restore path.

SQLite migration compatibility is forward-only. Keep the verified backup until the
new version has passed its observation window.

## Failure and chaos plan

Before declaring availability readiness, exercise and document these scenarios:

- unreachable, slow and malformed upstream answers; confirm bounded retries and
  failover without serving mismatched responses;
- disk-full and database-unavailable behavior; confirm no silent loss of zone state;
- restart during sustained DNS/HTTP traffic; confirm clean shutdown and recovery;
- rate-limit exhaustion and TCP connection exhaustion; confirm only bounded metrics
  and error responses, with no high-cardinality labels;
- network partition and node loss once the HA issues are implemented. The required
  consistency and split-brain contract belongs to #28 before replication work starts.
