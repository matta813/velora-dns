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
- [Query history, retention and filters](query-logging.md)
- [Architecture decisions](architecture/0001-foundation.md)
- [HA and failure-semantics decision](architecture/0004-high-availability.md)
- [DNS behavior and cache semantics](architecture/0002-dns-semantics.md)
- [Local zones architecture](architecture/0003-local-zones.md)
- [DHCP service boundaries](architecture/0005-dhcp-service-boundaries.md)
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
| `overview.png` | Dashboard overview with stats and query activity |
| `zones.png` | Local zone management (desktop) |
| `query-log.png` | Retained query filters and results (desktop) |
| `update-center.png` | Release discovery and update history (desktop) |
