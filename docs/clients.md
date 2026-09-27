# Clients and devices

Client definitions give addresses friendly names such as `Living Room TV` or
`Home Assistant`. Names appear in the query log, the dashboard's top clients and
anywhere else a client address is shown; the raw IP stays visible next to it.
Manage clients under **Network → Clients** or with the
[`/api/v1/clients` endpoints](api.md).

## Addresses and matching

A client owns 1–16 IP addresses or CIDR networks, for example `192.168.1.20`,
`192.168.1.0/24` or `2001:db8:1::/48`. IPv4-mapped IPv6 addresses match their
IPv4 form.

- When several networks contain an address, the **most specific** one wins: a
  client for `192.168.1.200` beats a client for `192.168.1.0/24`.
- Each address or network can belong to only one client, so matching is always
  deterministic. Client names are unique, ignoring case.
- Disabled clients keep their definition but no longer match anything.

Lookups use one hash probe per prefix length in use, so naming stays cheap even
with many clients and high query rates.

## Changing addresses

Velora identifies clients only by IP address; it does not collect MAC addresses,
host names or other identifiers from DNS traffic. When a device receives its
address from DHCP, give it a fixed address (for example with a reservation on the
**DHCP** page or on your router) before naming it, or name the whole network.

## Activity

When query logging is enabled, the Clients page shows how many retained queries
each client sent in the last 24 hours and when it was last seen, and lists busy
addresses without a name so you can name them directly. Activity is aggregated
on the server and bounded; with query logging disabled it is not shown.

Client definitions are stored in the database, survive restarts, and are the
building block for per-client policies.
