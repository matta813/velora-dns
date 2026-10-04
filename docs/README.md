# Documentation

- [Configuration reference](configuration.md)
- [Deployment, LAN setup and backups](deployment.md)
- [Encrypted backup and offline restore](backup.md)
- [Production assessment, load testing and recovery drills](production-assessment.md)
- [Encrypted DNS: DoT, DoH and DoQ](encrypted-dns.md)
- [DNSSEC validation](dnssec.md)
- [Development](development.md)
- [Validation evidence](validation.md)
- [REST API and metrics](api.md)
- [Local zones and record API](zones.md)
- [Clients and devices](clients.md)
- [Per-client filtering policies](policies.md)
- [Webhook notifications](webhooks.md)
- [Command palette and keyboard shortcuts](command-palette.md)
- [Clustering: primary and replicas](cluster.md)
- [DHCP pools, reservations and leases (no DHCP server runs yet)](dhcp.md)
- [Local DNS rewrites](rewrites.md)
- [Conditional forwarding and query precedence](forwarding.md)
- [Query history, retention and filters](query-logging.md)
- [Architecture decisions](architecture/0001-foundation.md)
- [HA and failure-semantics decision](architecture/0004-high-availability.md)
- [DNS behavior and cache semantics](architecture/0002-dns-semantics.md)
- [Local zones architecture](architecture/0003-local-zones.md)
- [DHCP service boundaries](architecture/0005-dhcp-service-boundaries.md)
- [Answer explanation](architecture/0006-answer-explanation.md)
- [Repository administration](administration.md)
- [Release preparation and recovery](releases.md)
- [Roadmap](roadmap.md)

This documentation describes the current development state. All implemented features
are documented; future features are listed in the roadmap.

## Screenshots

Screenshots of the management interface are in `assets/`:

They are generated with the deterministic demo fixtures in
`web/scripts/screenshot.mjs`. The fixtures use reserved documentation addresses
and contain no credentials, tokens, or production data.

| File | Description |
|---|---|
| `overview.png` | Dashboard overview with stats, query activity and upstream health |
| `overview-dark.png` | Dashboard overview in the dark theme |
| `overview-mobile.png` | Dashboard overview on a phone-sized screen |
| `navigation-mobile.png` | Grouped navigation drawer on a phone-sized screen |
| `sign-in.png` | Sign-in screen |
| `query-log.png` | Retained query filters and results |
| `events.png` | Event center with unread and acknowledged events |
| `zones.png` | Local zone management |
| `rewrites.png` | Local DNS rewrites with precedence hints |
| `forwarding.png` | Conditional forwarding rules with resolver health |
| `blocklists.png` | Blocklist sources with enable switches and status |
| `cache.png` | Cache usage and live cache entries |
| `clients.png` | Named clients with activity and unnamed addresses |
| `command-palette.png` | Command palette with page, action and resource results |
| `analytics.png` | Analytics: queries over time, response times and breakdowns |
| `policies.png` | Per-client filtering policies |
| `webhooks.png` | Webhook destinations with delivery status |
| `dhcp.png` | DHCP pools and active leases |
| `cluster.png` | Cluster primary with replicas and their sync status |
| `settings.png` | Preferences, server configuration and rate limiting (full page) |
| `update-center.png` | Release discovery and update history |
| `backup.png` | Encrypted backup creation, status and verification |
| `diagnostics.png` | Component health and support report |
| `audit-log.png` | Filterable audit trail of management actions |
