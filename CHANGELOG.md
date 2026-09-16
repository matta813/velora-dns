# Changelog

All notable changes are documented here. This project uses Semantic Versioning.

## Unreleased

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

- Node identity, membership and health monitoring with capability negotiation.
- Versioned configuration replication with SHA-256 hash verification.
- Zone replication between nodes with sync monitoring.
- Central cluster manager for multi-node management and config rollout.

### PostgreSQL cluster support

- PostgreSQL primary/replica cluster with configurable connection pooling.
- Cluster-aware read/write routing (writes to primary, reads from replica).
- Health checks for primary and replica connections.
- Node and config version persistence in cluster metadata tables.

### Zone import/export

- Zone file import (RFC 1035 format) via REST API and dashboard.
- Zone file export as `text/dns` download.

### API tokens

- Scoped API tokens with mandatory expiration (up to one year).
- Token creation, listing and revocation endpoints.
- Bearer token authentication via `Authorization: Bearer velora_<secret>`.
