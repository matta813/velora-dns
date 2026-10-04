# Roadmap

This is a directional plan, not a release schedule or a promise that every idea
will ship. **Now** contains tracked near-term priorities, not necessarily work
already in progress. **Next** is likely follow-on work; **Later** is longer-term.
**Exploring** needs design and validation before commitment. The
[issue tracker](https://github.com/matta813/velora-dns/issues) holds specifications
and progress; substantial new work should get its own issue before implementation.

## Now — tracked priorities

There are no open issues in the
[tracker](https://github.com/matta813/velora-dns/issues) as of 2026-10-04, so
nothing is tracked as near-term work. The items under **Next** are the candidates;
each needs an issue before implementation starts.

Recently completed: the per-language web UI translations
([#154](https://github.com/matta813/velora-dns/issues/154)). Each of the ten
languages has its own file under `web/src/i18n/`, missing keys fall back to
English, and `web/src/i18n.test.ts` checks key completeness and empty values.

## Next — likely follow-on work

### DNS and filtering

- Improve upstream health visibility and failover diagnostics; test behavior under
  slow, malformed, and unreachable upstreams.
- Show effective TTLs next to cached answers and explain which rule (rewrite,
  forwarding rule, client policy or blocklist) produced a given answer.

### Observability and security

- Make readiness, update failures, and DNS performance easier to diagnose through
  bounded metrics and actionable UI states.
- Expand audit coverage for sensitive management changes and test role boundaries,
  session handling, and secret redaction.

### Usability and developer experience

- Improve first-run onboarding and keep the browser and accessibility suites in
  step with new pages.
- Strengthen API documentation and CI coverage for deployment and upgrade paths.

## Later — planned direction

### Deployment and resilience

- Rehearse upgrade compatibility for SQLite and PostgreSQL deployments, and
  document a PostgreSQL backup path alongside the online SQLite restore.
- Continue systemd and Compose installer hardening, architecture support, and
  release artifact verification.

### DNS and integrations

- Extend zone and record workflows where operational gaps are demonstrated, with
  compatibility tests for DNSSEC and transfers.
- Improve management API integration paths and versioning guidance for external
  automation.

## Exploring — not committed

### High availability

- A single-writer primary/replica cluster keeps local zones in sync
  ([cluster setup](cluster.md)). Replicating settings, filtering and clients, and
  PostgreSQL cluster behavior, are not covered yet.
- Define failure, consistency, and recovery semantics before considering any
  quorum-based write or automatic failover design.

### Network services and extensibility

- DHCP: pool, reservation and lease management exists (API and web page), but the
  application does not start a DHCP listener, so no DHCP is served and DNS
  publishing is unused. Wiring a listener in, with the safety requirements of
  [ADR 0005](architecture/0005-dhcp-service-boundaries.md) and a separate
  deployment profile, is not committed. See [DHCP](dhcp.md).

## Completed work

The implemented DNS transports, local zones, filtering, cache, management API/UI,
roles, and deployment foundations are described in the
[README](../README.md) and [architecture documentation](architecture/0001-foundation.md).
See [GitHub Releases](https://github.com/matta813/velora-dns/releases) and the
[changelog](../CHANGELOG.md) for release history. Completed work is intentionally
kept out of the active sections above.
