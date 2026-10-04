# DHCP

> **Status: not a working DHCP service.** Velora DNS can store DHCP pools,
> reservations and leases, and the API and the **DHCP** page manage them. **No part
> of the application starts a DHCP listener**, so nothing answers DHCP requests,
> no leases are created, and no DNS records are published from leases. Do not
> replace your router's or another server's DHCP with it.

See [ADR 0005](architecture/0005-dhcp-service-boundaries.md) for the target design
and the full list of what is and is not implemented.

## What exists

- Database tables for pools, reservations and leases (migration `012_dhcp.sql`),
  in the management database (SQLite or PostgreSQL).
- Management API under `/api/v1/dhcp/` and the **DHCP** page in the web interface.
  The API reports the `dhcp` capability when the store is present.
- A DHCPv4 server library in `internal/dhcp` (packet parsing, pool allocator,
  UDP server). It is exercised by tests only, including packet-level tests with a
  real UDP client (`tests/dhcp_integration_test.go`); `internal/app` never creates it.

## Configuration

These keys exist in the configuration (`dhcp:` in YAML) and are validated, but they
**do not start anything**:

| Key | Environment variable | Default | Effect today |
|---|---|---|---|
| `dhcp.enabled` | `VELORA_DHCP_ENABLED` | `false` | When true, `dhcp.listen` is validated and checked for conflicts with other listeners. No server starts. |
| `dhcp.listen` | `VELORA_DHCP_LISTEN` | `0.0.0.0:67` when enabled and empty | Validated only. The library itself ignores it and binds `0.0.0.0:67` unless a test overrides the address. |
| `dhcp.publish_dns` | `VELORA_DHCP_PUBLISH_DNS` | `false` | None. Nothing reads it. |

`VELORA_DHCP_ENABLED` and `VELORA_DHCP_PUBLISH_DNS` must be boolean values or
configuration loading fails.

## Pools and reservations (data only)

A pool has a name (unique), interface name, IPv4 subnet, gateway, DNS servers and a
lease time. Validation requires a name, an interface, a valid subnet, a gateway
inside the subnet and `lease_seconds` between 60 and 31536000. Creating a pool via
the API sets it enabled. There is no address range: if a server were running, every
host address of the subnet except the gateway would be a candidate. The interface
name is stored but not used for binding.

A reservation maps a MAC address (6 bytes) to an IP in a pool, optionally with a
hostname; MAC and IP are each unique per pool. The API validates only the MAC; the
database enforces uniqueness, and the IP is not checked against the pool subnet by
the API.

## API

All endpoints require authentication. Read endpoints are open to any signed-in
role; changing endpoints (POST, PUT, DELETE) are rejected for the `viewer` role and
for tokens without the `write` or `admin` scope, need the CSRF token for browser
sessions, and are written to the audit log. DHCP endpoints are not admin-only.

| Method and path | Purpose |
|---|---|
| `GET /api/v1/dhcp/pools` | List pools |
| `POST /api/v1/dhcp/pools` | Create a pool |
| `GET`, `PUT`, `DELETE /api/v1/dhcp/pools/{id}` | Read, replace, delete a pool (deleting cascades to its reservations and leases) |
| `GET`, `POST /api/v1/dhcp/pools/{id}/reservations` | List or create reservations |
| `DELETE /api/v1/dhcp/reservations/{id}` | Delete a reservation |
| `GET /api/v1/dhcp/leases` | Active leases, or all leases of one pool with `?pool_id=` |
| `DELETE /api/v1/dhcp/leases/{id}` | Delete a lease record |

The shapes are in the [OpenAPI contract](openapi.json). Because no server runs, the
lease list stays empty unless rows are created some other way.

## Privileges, if a listener is ever wired in

DHCP servers receive client broadcasts on UDP port 67, a privileged port. A process
would need to bind it (root or `CAP_NET_BIND_SERVICE`) and be on the same layer-2
network as the clients, or receive relayed traffic. The library replies to the
sender's source address; it does not use raw sockets or bind to an interface.

## Why the default container cannot serve DHCP

The shipped `Dockerfile` runs as the non-root user `10001` and exposes only
5353/udp, 5353/tcp and 8080/tcp. `docker-compose.yml` sets `cap_drop: [ALL]`,
`no-new-privileges`, a read-only root file system, and publishes only the DNS and
HTTP ports on a bridge network. UDP/67 is not published, no capability to bind it
is granted, and a bridge network does not carry client broadcasts. A DHCP profile
would need host or macvlan networking and `CAP_NET_BIND_SERVICE`; none is provided
and the default must not gain those privileges (ADR 0005).

## DNS publishing

There is none. `dhcp.publish_dns` has no effect, the application passes no DNS
callback to the library, and DNS queries never create or change leases.

## Known limits

- No listener is started; no DHCP is served.
- One pool per server instance in the library; no subnet selection and no
  interface allowlist. The library drops relayed packets (non-zero `giaddr`).
- The library saves the lease before sending the ACK; if the save fails it sends
  nothing, but the in-memory allocation stays.
- Dynamic offers reserve the address in memory without a timeout until the client
  confirms or releases it. Expired stored leases are never cleaned up:
  `ExpireLeases` (store and allocator) has no caller.
- DECLINE and RELEASE mark the client's stored lease `released` and free the
  in-memory allocation. There is no DECLINE quarantine or probing, so a declined
  address can be offered again at once (a skipped test records this).
- A REQUEST with the address in `ciaddr` is renewed and keeps its address. REBINDING
  is not handled as a separate state, INFORM is ignored, and replies are unicast to the
  packet's source address (no broadcast replies).
- `dhcp_leases` is unique on pool and IP, not pool and MAC. A client that gets a
  different address leaves its old row active, and lookups by MAC may return either row.
- Changes made through the API are not seen by an already built allocator.
- T1 and T2 are 50% and 87.5% of the lease time. The domain-name option is not
  sent. INFORM is ignored. Only IPv4.
- Backward clock changes are not guarded.
