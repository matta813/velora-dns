# Changelog

All notable changes are documented here. This project uses Semantic Versioning.

## Unreleased

- Answer explanation: Diagnostics shows which stage answers a name, type and optional client (blocklist or client policy, rewrite, local zone, cache with remaining TTL, conditional forwarding rule or global upstreams) with the rule behind it, through a side-effect-free `POST /api/v1/diagnostics/explain` that shares the resolver's decision code, never queries an upstream and is open to viewers.
- DHCP documentation made accurate: pools, reservations and leases can be stored and managed through the API and web page, but the application does not start a DHCP listener, so no DHCP is served and `dhcp.publish_dns` has no effect. ADR 0005 now lists what is implemented and what is not, a new DHCP page documents the limits, and the roadmap and production assessment mark DHCP as not implemented (#22). Packet-level tests now exercise the library with a real UDP client (DECLINE quarantine and interface, broadcast and subnet selection are documented skipped tests), and the library's DHCPv4 wire handling, renewals, release/decline persistence, reserved-address protection and stored-lease expiry comparison were corrected; the library is still not started by the application.
- Subtle motion: pages, menus, notices and the command palette fade or slide a few pixels into place, buttons respond to presses, switches, chevrons and progress bars ease between states, and all of it is off when the system prefers reduced motion (#243).
- End-to-end tests: Go workflows for backup create/change/restore across a restart, the updater against a local fake agent, rate limiting and two-process cluster replication, plus a Playwright browser suite against a real server (sign-in, zones with DNS answers, query log, blocking, command palette, config errors, roles, axe, mobile) and a CI job that also runs the page-by-page accessibility audit (#179).
- Clustering from the web interface: create a primary, invite replicas with one-time join tokens, and keep local authoritative zones identical through signed, authenticated snapshots; replicas reject local zone edits, the primary shows each replica's sync status, and nodes can be removed, leave or dissolve the cluster (#215).
- Command palette (Ctrl/⌘+K): jump to pages, run safe role-aware actions and find zones, records, clients, rewrites, forwarding rules and blocklists through a bounded, debounced server search, with full keyboard and screen reader support (#255).
- Accessibility and mobile: WCAG AA contrast in light and dark themes, 24 px minimum targets (40 px on touch screens), no horizontal page scrolling on phones, keyboard-operable navigation dropdowns, a focus-trapped mobile menu, focus moved to the page heading on navigation, focusable scrollable tables and the document language set for screen readers; a repeatable axe-core audit script covers every page (#251).
- Restore from the web interface: upload and check an encrypted backup (metadata, contents, compatibility warnings) without changing anything, then confirm to restart and apply it with a safety copy; the restored start is confirmed once ready and rolled back automatically if it never gets there (#172).
- Configuration validation reports every invalid field with its path (API `fields`, highlighted in Settings), detects conflicting listeners, rejects unsafe paths and invalid filtering domains, and offers a dry-run `POST /api/v1/config/validate`; tests cover rollback failure, readiness rollback and concurrent changes (#175).
- Simpler Updates page: one Check for updates button, an Update button only when a newer version exists, live progress through the restart, clear errors, the new version shown on completion, and a server-side guard against concurrent update requests (#244).
- Analytics page: queries, blocked, cached and failed answers over 1 hour to 30 days, upstream response-time trend, query types, response codes, answer sources, upstream usage and top blocked domains, aggregated in the database from retained query history (#253).
- Webhook notifications: send upstream, blocklist, backup and configuration events to HTTP endpoints with a versioned JSON payload, per-webhook event and severity filters, optional bearer token, background delivery with retries, a test action and SSRF protection (#252).
- Per-client filtering policies: a named client can follow global filtering, bypass it, or use its own blocklists plus extra allowed and blocked domains; the most specific client network decides, and a lookup shows which policy applies to an address (#249).
- Clients and devices: name IP addresses and networks (most specific wins), show names in the query log and top clients, and see 24-hour activity plus unnamed busy addresses (#254).
- Local DNS rewrites: answer names or `*.parent` wildcards with fixed A, AAAA or CNAME data ahead of local zones, with documented precedence and hints when a blocklist or zone is involved (#247).
- Conditional forwarding: send queries for chosen domains to dedicated resolvers, with most-specific matching, per-rule health, loop protection, a test action and cache invalidation on change (#246).
- Scheduled blocklist updates: URL sources refresh automatically every hour to weekly, failures keep the previous list and retry with backoff, and the dashboard shows last attempt, next update and failure state (#250).
- Cache inspection: search cached answers by name and record type, remove a single entry or every answer for a domain (optionally with subdomains), and confirm before flushing the whole cache (#248).
- Reworked web interface in the style of established self-hosted DNS dashboards: top header with horizontal navigation and dropdown groups, flat bordered cards, dense tables, colored stat cards, top clients/domains with request shares, a general statistics table, and matching light and dark themes.

- Opt-in bounded query history, filtered API and dashboard, loss metrics and graceful drain.

- Authoritative local zones with transactional SQLite persistence, SOA/NS and CNAME resolution.
- Revision-protected zone and record REST endpoints and responsive dashboard management.

- Repository foundation, contribution guidelines, and phased roadmap.

- Independent UDP/TCP DNS forwarding with retry, failover, TCP fallback and client ACLs.
- Bounded in-memory positive-answer cache with TTL aging, expiry and flush.
- Strict YAML/environment configuration, structured logging and graceful lifecycle.
- SQLite migration foundation and operational REST/health/metrics endpoints.
- Responsive React overview, cache management and read-only settings.
- Non-root Docker deployment, persistent volume and protected CI workflow.
- Administrative baseline, grouped Dependabot and disabled release automation.

### Encrypted DNS transports

- DNS-over-TLS (DoT) listener and upstream support with TLS 1.2+ and certificate reload.
- DNS-over-HTTPS (DoH) listener and upstream support (GET base64url, POST dns-message).
- DNS-over-QUIC (DoQ) listener implementing RFC 9250 with stream-based DNS queries.

### DNSSEC validation

- Opt-in local DNSSEC validation with DS/DNSKEY chain verification to operator-managed trust anchors.
- NSEC/NSEC3 negative answer proof validation, signature lifetime checking.
- AD/CD/DO bit handling, trust anchor rollover support.

### Authentication and access control

- User management with admin/operator/viewer roles.
- Session-based authentication with HttpOnly SameSite=Strict cookies and CSRF protection.
- Scoped API tokens (read/write/admin) with SHA-256 digest storage and mandatory expiration.
- Audit logging for mutations and authentication events.

### PostgreSQL storage adapter

- PostgreSQL driver support alongside SQLite with dialect-aware query placeholders.
- `database_driver` and `database_url` configuration for driver selection.
- Transactional migrations adapted for PostgreSQL RETURNING clause.

### Secondary zones and transfers

- TSIG authentication with HMAC-SHA256/1/512 key storage and sign/verify.
- AXFR/IXFR transfer client over TCP with TSIG-signed packets.
- Secondary zone manager with SOA lifecycle, serial tracking and refresh scheduling.
- REST API for TSIG key management (list, create, delete) and secondary zone creation.

### DNS-over-QUIC

- DNS-over-QUIC (DoQ) server implementing RFC 9250.
- Stream-based DNS query handling over QUIC transport.
- Integration with existing ACL, rate limits and resolver pipeline.

### Multi-node and cluster management

- Internal scaffolding for node identity, membership, versioned configuration,
  zone replication and central management. These packages are not yet connected
  to the production runtime and are not an available deployment mode.

### PostgreSQL cluster support

- Internal scaffolding for primary/replica connection pooling, routing and health
  checks. Production PostgreSQL currently uses the single configured database URL.

### Zone import/export

- Zone file import (RFC 1035 format) via REST API and dashboard.
- Zone file export as `text/dns` download.

### API tokens

- Scoped API tokens with mandatory expiration (up to one year).
- Token creation, listing and revocation endpoints.
- Bearer token authentication via `Authorization: Bearer velora_<secret>`.
