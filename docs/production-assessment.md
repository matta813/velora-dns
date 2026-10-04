# Production assessment

Assessed on 2026-09-16 against the current development baseline. Re-checked on
2026-10-04 against the code, Makefile, CI workflows and docs: only the Availability
row, the first checklist item and the multi-node checklist item were corrected; the
other rows were not re-assessed. This is an
operational readiness assessment, not a production approval: Velora DNS must not
be advertised as production-ready until every blocking item below is resolved and
the stated drills are performed in the target environment.

## Decision

| Area | Status | Evidence and action |
|---|---|---|
| Management access | Conditional | Users, roles (admin/operator/viewer), session auth and scoped API tokens are implemented. Ensure management HTTP binds only to loopback or an authenticated tunnel. |
| Resolver transport | Conditional | UDP/TCP/DoT/DoH/DoQ DNS transports are implemented. Use a trusted private network and firewall; validate encrypted transport configuration. |
| DNS correctness | Conditional | Local integration tests cover supported record types, TCP fallback, EDNS, cookies, cache semantics, DNSSEC validation and TSIG-signed transfers. Production anchor lifecycle exercises remain an operator responsibility. |
| Abuse resistance | Conditional | Global/client query limits, concurrent-work and TCP-connection caps are configured. Size them with the procedure below and monitor rejection metrics. |
| Data durability | Conditional | SQLite and PostgreSQL persistence, migrations and WAL checkpointing are tested. Operators must complete a backup/restore drill before use. |
| DHCP | Not implemented | Pools, reservations and leases can be stored and managed, but the application never starts a DHCP listener, so no DHCP is served and `dhcp.publish_dns` has no effect. The library has lease-before-ACK, fail-closed saves, relay rejection and packet-level tests, but the interface allowlist, DECLINE quarantine, subnet selection, expired-lease cleanup and clock-backward protection are not implemented ([DHCP](dhcp.md)). Keep DHCP on your router or a dedicated server. |
| Availability | Not implemented | A single-writer primary/replica cluster replicates local zones only ([cluster setup](cluster.md)). There is no automatic failover, and settings, filtering, clients and PostgreSQL cluster behavior are not replicated or covered. Treat a cluster as zone redundancy, not high availability; the primary is a single point of failure for changes. |

## Security review checklist

Before deployment, record the reviewer, date and result for each control:

- Run `make check` (format, vet, golangci-lint, race tests, frontend lint/types/tests,
  builds, release-automation tests and the Compose configuration check) on the
  release candidate, plus `make test-e2e` and the browser suite described in
  [development](development.md).
- Ensure management HTTP binds only to loopback; permit remote access only through
  an authenticated tunnel or a separately authenticated reverse proxy.
- Restrict `dns.allowed_clients` to actual internal CIDRs. Do not operate this
  recursive resolver publicly.
- Set global and per-client QPS, burst, concurrent-work and TCP-connection limits
  explicitly. Alert on `dns_overload_rejections_total`; its labels are bounded and
  intentionally contain no client identity.
- Use private, permission-restricted persistent storage. Query history can contain
  client addresses and queried names; set retention to the minimum operational need.
- Verify the image digest and SBOM/dependency scan results in CI. Do not enable
  release publication while `RELEASE_ENABLED` remains disabled.
- For PostgreSQL deployments, restrict network access to the database and use TLS
  connections. Validate cluster health checks and replica routing.
- Do not rely on Velora for DHCP: no listener is started by the application. Leave
  `dhcp.enabled` and `dhcp.publish_dns` unset, and treat the DHCP page as data
  management only.
- Before any future DHCP listener is accepted, require the ADR 0005 items still
  missing: interface allowlist and subnet selection, DECLINE quarantine, expired-lease
  cleanup, clock-backward protection and packet-level tests
  on a real network namespace, with a separate non-default container profile.
- Do not rely on a multi-node topology for availability: only zone replication from
  one primary exists, with no failover or consistency guarantees beyond that
  ([cluster setup](cluster.md)).

## DNS compliance regression suite

The automated suite is the repeatable protocol evidence, not a claim of universal
DNS interoperability. Run it before every candidate:

```bash
go test ./internal/dns ./tests
```

It covers UDP/TCP/DoT/DoH/DoQ listener behavior, supported IN record classes, local-zone
SOA/NXDOMAIN/NODATA behavior, upstream failover/retries, response/question and
CNAME-chain validation, EDNS size and privacy policy, DNS Cookies, cache TTL
semantics, DNSSEC validation, TSIG authentication and AXFR/IXFR zone transfers.

## Sustained-load procedure and resource sizing

Benchmark only against a private test network and local upstream fixture; never load
test public resolvers. Capture the exact config, hardware, Go version, duration,
query mix, packet loss and results alongside the deployment record.

1. Start with `global_qps` at 60% of the measured sustainable QPS and `client_qps`
   at the largest justified client share; make `rate_limit_burst` no larger than one
   second of that client allowance.
2. Hold mixed UDP/TCP/DoT/DoH traffic for at least 30 minutes at 50%, 75% and 100% of the
   proposed global limit. Include cache hits, cache misses, blocked names and large
   answers that trigger TCP fallback.
3. Pass only if DNS latency remains within the deployment SLO, memory reaches a
   stable plateau, file descriptors remain below 70% of the process limit, and no
   unexpected `dns_overload_rejections_total` or query-log drops occur.
4. Repeat with intentionally abusive UDP/TCP traffic. Expected behavior is bounded
   memory and connections plus increasing rejection counters, not unbounded latency.

The initial container limit of 256 MiB is a development baseline, not a sizing
recommendation. Reserve headroom for SQLite/PostgreSQL, query logging, cache entries, TCP
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
new version has passed its observation window. PostgreSQL migrations are also
forward-only; ensure backups exist before applying schema changes.

## Failure and chaos plan

Before declaring availability readiness, exercise and document these scenarios:

- unreachable, slow and malformed upstream answers; confirm bounded retries and
  failover without serving mismatched responses;
- disk-full and database-unavailable behavior; confirm no silent loss of zone state;
- restart during sustained DNS/HTTP traffic; confirm clean shutdown and recovery;
- rate-limit exhaustion and TCP connection exhaustion; confirm only bounded metrics
  and error responses, with no high-cardinality labels;
- network partition and primary or replica loss in a cluster (a replica keeps serving
  its last applied zones; confirm sync recovers after healing). Failover is not
  implemented, so there is no automatic recovery to test.
