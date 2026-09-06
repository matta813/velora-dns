# Roadmap

Only checked items are implemented. Milestones describe targets, not release promises.

## Phase 1 — MVP

- [ ] UDP/TCP DNS and upstream forwarding
- [ ] In-memory TTL cache
- [ ] Local authoritative zones and record CRUD
- [ ] Blocklists, allowlists, wildcard matching, remote list updates
- [ ] Query logging, retention, filters, top domains and clients
- [ ] Versioned REST API
- [ ] React management dashboard
- [ ] SQLite persistence
- [ ] Docker deployment
- [ ] Prometheus metrics

## Phase 2

- [ ] DNS-over-TLS
- [ ] DNS-over-HTTPS
- [ ] DNSSEC validation
- [ ] Users and roles
- [ ] PostgreSQL storage adapter
- [ ] Zone import/export

## Phase 3

- [ ] DHCP server
- [ ] DNS-over-QUIC
- [ ] Secondary zones
- [ ] AXFR/IXFR
- [ ] TSIG
- [ ] API tokens

## Phase 4

- [ ] High availability
- [ ] Multiple DNS nodes
- [ ] Configuration replication
- [ ] Zone replication
- [ ] Central management
- [ ] PostgreSQL cluster support

## Milestones

- **v0.1.0 – Foundation:** config, lifecycle, forwarding, cache, initial API/UI, CI.
- **v0.2.0 – DNS MVP:** local zones, filtering, query logging, protocol hardening.
- **v0.3.0 – Management:** full CRUD UI, users/roles, import/export.
- **v0.4.0 – Production Preview:** encrypted DNS, DNSSEC, security assessment.
- **v1.0.0:** stable storage/API contracts and operational validation. HA remains a longer-term track.
