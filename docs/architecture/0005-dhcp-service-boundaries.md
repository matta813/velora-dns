# ADR 0005: DHCP service boundaries

## Status

Accepted design for issue #22; no DHCP listener exists yet.

## Decision

DHCP is separately enabled with its own UDP listener, lease store and
configuration. It must not share DNS listener state or SQLite tables implicitly.
A lease commits before ACK; startup reconstructs leases and refuses expired or
conflicting addresses. DHCP owns pools, client identifiers, reservations and
DECLINE quarantine.

DNS integration is one-way and best-effort: a durable lease transition may publish
a validated local DNS record. A DNS failure never changes the DHCP outcome; it
emits a retryable event. DNS queries never create or alter leases.

## Network and privilege model

DHCP uses UDP/67 and broadcasts. Its dedicated deployment profile requires an
explicit interface allowlist and any required bind/raw-network capability; the
default container must not gain those privileges. The service validates receiving
interface, server identifier and subnet/pool selection, and rejects relay traffic
until relay support is explicitly designed.

## Conflict and recovery policy

Allocate only addresses outside non-expired leases and reservations. Probe before
ACK where supported; conflicts and DECLINEs enter a bounded quarantine. Database
failure fails closed (no ACK); DNS-update failure occurs only after the durable
lease commit. Backward clock movement must not extend a persisted lease expiry.

## Packet-level test plan

Namespace integration tests with a real UDP client/server must cover
DISCOVER/OFFER/REQUEST/ACK, renew/rebind, release, NAK, DECLINE, reservations,
pool exhaustion, restart and storage failure. Assert options 1, 3, 6, 51, 53 and
54; broadcast/unicast behavior; subnet selection; malformed packet rejection; and
no allocations on an unapproved interface. DNS tests must prove committed lease
publication and preserve the lease when publication fails.

DHCPv6, PXE, relays, HA lease replication and dynamic DNS updates are excluded
until separate protocol, authentication and consistency decisions exist.
