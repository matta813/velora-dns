# ADR 0005: DHCP service boundaries

## Status

Accepted as the target design for issue #22. **Only part of it is implemented.**
Pool, reservation and lease data, the management API and the web page exist, and
`internal/dhcp` contains a DHCPv4 server library. Nothing in the application starts
that library, so the running product does not answer DHCP. Several safety
requirements below are not implemented (DECLINE quarantine, interface allowlist, subnet
selection, probing, clock protection); relay rejection and lease-before-ACK exist in the library only. See
[Implemented and not implemented](#implemented-and-not-implemented) and
[DHCP](../dhcp.md).

## Decision

The target design is: DHCP is separately enabled with its own UDP listener, lease store and
configuration. It must not share DNS listener state or SQLite tables implicitly.
A lease commits before ACK; startup reconstructs leases and refuses expired or
conflicting addresses. DHCP owns pools, client identifiers, reservations and
DECLINE quarantine.

DNS integration is one-way and best-effort: a durable lease transition may publish
a validated local DNS record. A DNS failure never changes the DHCP outcome; it
emits a retryable event. DNS queries never create or alter leases.

## Network and privilege model

The target design is: DHCP uses UDP/67 and broadcasts. Its dedicated deployment profile requires an
explicit interface allowlist and any required bind/raw-network capability; the
default container must not gain those privileges. The service validates receiving
interface, server identifier and subnet/pool selection, and rejects relay traffic
until relay support is explicitly designed.

## Conflict and recovery policy

Target policy: allocate only addresses outside non-expired leases and reservations. Probe before
ACK where supported; conflicts and DECLINEs enter a bounded quarantine. Database
failure fails closed (no ACK); DNS-update failure occurs only after the durable
lease commit. Backward clock movement must not extend a persisted lease expiry.

## Packet-level test plan

Target plan: namespace integration tests with a real UDP client/server must cover
DISCOVER/OFFER/REQUEST/ACK, renew/rebind, release, NAK, DECLINE, reservations,
pool exhaustion, restart and storage failure. Assert options 1, 3, 6, 51, 53 and
54; broadcast/unicast behavior; subnet selection; malformed packet rejection; and
no allocations on an unapproved interface. DNS tests must prove committed lease
publication and preserve the lease when publication fails.

DHCPv6, PXE, relays, HA lease replication and dynamic DNS updates are excluded
until separate protocol, authentication and consistency decisions exist.

## Implemented and not implemented

Verified against the code, not the intent above.

| Requirement | State | Detail |
|---|---|---|
| Pool, reservation and lease tables | Implemented | `dhcp_pools`, `dhcp_reservations`, `dhcp_leases` in migration `012_dhcp.sql`; SQL in `internal/database/dhcp.go`. |
| Management API and web page | Implemented | `/api/v1/dhcp/*` is registered when the DHCP store is present; the UI has a DHCP page. |
| DHCPv4 packet library and server | Implemented as library only | `internal/dhcp` handles DISCOVER, REQUEST, RELEASE and DECLINE over UDP. No production code calls `dhcp.NewServer`. |
| Separately enabled own UDP listener | Not implemented | `dhcp.enabled`, `dhcp.listen` and `dhcp.publish_dns` are parsed and validated (and the listen address takes part in the listener-conflict check) but nothing reads them to start a server. Enabling them changes nothing at runtime. |
| Own lease store, not shared with other state | Not implemented | Leases live in the same management database as every other table. |
| Lease commits before ACK | Implemented in the library only | `handleRequest` saves the lease and only then sends the ACK (`internal/dhcp/server.go`). Moot while no server runs. |
| Fail closed on database failure | Partial, library only | A failed lease save withholds the ACK (no NAK is sent), covered by a packet-level test. The in-memory allocation made earlier is not rolled back. Offers (DISCOVER) do not touch the store. |
| Startup reconstructs leases, refuses expired or conflicting ones | Partial (library) | `NewPoolAllocator` seeds from the lease list passed in and skips expired entries; the application never does this. |
| Reservations and client identifiers | Partial | Reservations are honoured by the allocator by MAC. The client identifier is stored on the lease but not used to identify the client. |
| Allocation avoids non-expired leases and reservations | Partial | Dynamic allocation and requested addresses avoid IPs leased in memory to other MACs and addresses reserved for other MACs. Offers are held in memory only, with no timeout, so an unconfirmed OFFER keeps its address until released. No quarantine, no probing. |
| Probe before ACK | Not implemented | No ICMP/ARP probe. |
| DECLINE quarantine, bounded conflict quarantine | Not implemented | DECLINE logs, marks the client's stored lease `released` (when it is active and holds that address) and frees the in-memory allocation, so the address can be handed out again at once. A skipped test (`TestDHCPDeclineQuarantinesAddress`) records the missing hold-down. |
| Interface allowlist, receiving interface validation | Not implemented | The pool `interface` field is stored and required but never used; the socket binds `0.0.0.0:67` on all interfaces. Skipped test `TestDHCPBroadcastAndInterfaceSelection`. |
| Server identifier and subnet/pool selection | Partial | A REQUEST with a different server identifier is ignored. A server handles exactly one pool; there is no subnet selection (skipped test `TestDHCPBroadcastAndInterfaceSelection`). Replies go to the packet's source address; broadcast replies (broadcast flag, `255.255.255.255`) are not implemented. |
| Reject relay traffic | Implemented in the library only | `Server.handle` drops packets with a non-zero `giaddr` and non-BOOTREQUEST packets (`internal/dhcp/server.go`); malformed packets (no magic cookie, truncated options, hardware length above 16) are rejected by the parser. Covered by a packet-level test. Moot while no server runs. |
| Dedicated privilege profile; default container unchanged | Partially true | The default container drops all capabilities and publishes no UDP/67 port, so it cannot serve DHCP; no dedicated DHCP deployment profile exists. |
| Backward clock movement must not extend a lease | Not implemented | Expiry is `time.Now()` plus the lease time with no monotonic or persisted-time check. |
| DNS publication after durable commit, failure does not change the outcome | Not implemented | The library calls an optional callback after the lease save and ACK, but it is synchronous with no error return or retry event; the application passes none and `publish_dns` is unused. No DNS record is ever created from a lease. |
| Expiry of old leases | Partial | `Store.ExpireLeases` now compares UTC RFC3339 timestamps correctly (same-day leases expire; tested), but `Store.ExpireLeases` and `PoolAllocator.ExpireLeases` have no caller, so stored leases are never marked expired automatically. |
| One lease row per client | Not implemented | `dhcp_leases` is unique on (pool, IP), not (pool, MAC). A client that is given a different address leaves its previous row active, and lookups by MAC then return an arbitrary one of the rows. Not covered by a test. |
| Renew, rebind, INFORM | Partial | A REQUEST carrying the address in `ciaddr` (no option 50) is treated as a renewal and keeps the client's address; this works after a restart too (tested). REBINDING is not a separate state (a broadcast REQUEST is handled like any REQUEST), and INFORM is ignored. T1 and T2 are sent as 50% and 87.5% of the lease time. |
| Packet-level integration tests | Implemented, with documented gaps | `tests/dhcp_integration_test.go` drives the library with a real UDP client: DISCOVER/OFFER/REQUEST/ACK with options 1, 3, 6, 51, 53, 54, T1/T2, renew, release, NAK, reservation, pool exhaustion, restart, malformed and relayed packets, storage failure and publication after commit. Two tests are skipped with reasons: DECLINE quarantine, and broadcast/interface/subnet selection (needs a network namespace). They run against the library on loopback, not a listener started by the application. |

DHCPv6, PXE, relays, HA lease replication and dynamic DNS updates remain excluded.
