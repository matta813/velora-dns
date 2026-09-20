# Roadmap

This is a directional plan, not a release schedule or a promise that every idea
will ship. **Now** contains tracked near-term priorities, not necessarily work
already in progress. **Next** is likely follow-on work; **Later** is longer-term.
**Exploring** needs design and validation before commitment. The
[issue tracker](https://github.com/matta813/velora-dns/issues) holds specifications
and progress; substantial new work should get its own issue before implementation.

## Now — tracked priorities

### Updates and operations

- Complete the UI-driven update flow for systemd and Docker Compose: release
  discovery by channel, trusted download and checksum verification, real updater
  execution, progress, diagnostics, and rollback. See [#156](https://github.com/matta813/velora-dns/issues/156).

### Web UI and internationalisation

- Split translations into per-language files, retain a fallback, and test key
  completeness. See [#154](https://github.com/matta813/velora-dns/issues/154).

## Next — likely follow-on work

### DNS and filtering

- Improve upstream health visibility and failover diagnostics; test behavior under
  slow, malformed, and unreachable upstreams.
- Add clearer cache inspection and controls, including visibility into effective
  TTLs and safe invalidation; review conditional forwarding and local rewrites as
  separate proposals.
- Improve per-client filtering policy and blocklist update diagnostics without
  exposing query data unnecessarily.

### Observability and security

- Make readiness, update failures, and DNS performance easier to diagnose through
  bounded metrics and actionable UI states.
- Expand audit coverage for sensitive management changes and test role boundaries,
  session handling, and secret redaction.

### Usability and developer experience

- Improve mobile layout, keyboard accessibility, onboarding, and form validation
  across the management UI.
- Strengthen API documentation, local test fixtures, and CI coverage for deployment
  and upgrade paths.

## Later — planned direction

### Deployment and resilience

- Improve backup/restore tooling and rehearse upgrade compatibility for SQLite and
  PostgreSQL deployments before expanding production guidance.
- Continue systemd and Compose installer hardening, architecture support, and
  release artifact verification.

### DNS and integrations

- Extend zone and record workflows where operational gaps are demonstrated, with
  compatibility tests for DNSSEC and transfers.
- Improve management API integration paths and versioning guidance for external
  automation.

## Exploring — not committed

### High availability

- Evaluate multi-node membership, configuration and zone replication, central
  management, and PostgreSQL cluster behavior. Foundation packages exist, but
  these capabilities are **not connected to the production runtime**; single-node
  deployment remains the supported model. See the
  [production assessment](production-assessment.md).
- Define failure, consistency, and recovery semantics before considering any
  quorum-based write or automatic failover design.

### Network services and extensibility

- Assess whether DHCP, webhooks, and additional language packs belong in the core
  product or should remain separate integrations.

## Completed work

The implemented DNS transports, local zones, filtering, cache, management API/UI,
roles, and deployment foundations are described in the
[README](../README.md) and [architecture documentation](architecture/0001-foundation.md).
See [GitHub Releases](https://github.com/matta813/velora-dns/releases) and the
[changelog](../CHANGELOG.md) for release history. Completed work is intentionally
kept out of the active sections above.
