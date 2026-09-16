# Roadmap

Only checked items are implemented. Milestones describe targets, not release promises.

## Phase 1 — MVP

- [x] UDP/TCP DNS and upstream forwarding
- [x] In-memory TTL cache
- [x] Local authoritative zones and record CRUD
- [ ] Blocklists, allowlists, wildcard matching, remote list updates
- [x] Query logging, bounded retention and filters
- [x] Top domains and clients
- [x] Versioned operational API (status/stats/cache/config)
- [ ] Full management CRUD API
- [x] React overview, cache and settings
- [x] Zones management screen
- [x] Query history screen
- [ ] Complete blocklist management controls
- [x] SQLite management persistence (migrations, zones and records)
- [x] Docker deployment
- [x] Prometheus metrics

## Phase 2

- [x] DNS-over-TLS
- [x] DNS-over-HTTPS
- [x] DNSSEC validation
- [x] Users and roles
- [x] PostgreSQL storage adapter
- [x] Zone import/export

## Phase 3

- [ ] DHCP server
- [x] DNS-over-QUIC
- [x] Secondary zones
- [x] AXFR/IXFR
- [x] TSIG
- [x] API tokens

## Phase 4

- [x] High availability (foundation)
- [x] Multiple DNS nodes
- [x] Configuration replication
- [x] Zone replication
- [x] Central management
- [x] PostgreSQL cluster support

## Milestones

- **v0.1.0 – Foundation:** config, lifecycle, forwarding, cache, initial API/UI, CI.
- **v0.2.0 – DNS MVP:** local zones, filtering, query logging, protocol hardening.
- **v0.3.0 – Management:** full CRUD UI, users/roles, import/export.
- **v0.4.0 – Production Preview:** encrypted DNS, DNSSEC, security assessment.
- **v0.5.0 – Transports and Authority:** DoT/DoH/DoQ, secondary zones, TSIG, AXFR/IXFR.
- **v0.6.0 – Multi-Node:** node membership, config/zone replication, central management, PG cluster.
- **v1.0.0:** stable storage/API contracts and operational validation. Full HA with quorum-based writes remains a longer-term track.
