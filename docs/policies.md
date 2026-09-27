# Per-client filtering policies

A filtering policy changes how blocking applies to one [client](clients.md).
Clients without a policy, and addresses that match no client, use the global
blocklists and allowlists.

## Modes

| Mode | Effect |
| --- | --- |
| `default` | Global filtering, the same as having no policy. Useful to pause a custom policy without losing it. |
| `disabled` | No filtering at all for this client, including the `filtering.blocklist` configuration rules. |
| `custom` | Only the blocklists chosen for this policy, plus the policy's extra blocked and allowed domains and the configuration allow/block rules. |

A custom policy may choose blocklists that are disabled globally, so you can
keep a strict list (for example for children's devices) that does not affect
the rest of the network. Extra domains also match their subdomains, and an
allowed domain always wins over any blocklist.

A policy can be switched off with `enabled: false`; the client then uses global
filtering until it is switched on again.

## Which policy applies

The querying address is matched against client definitions and the most
specific network wins, exactly as for client names. A device at
`192.168.1.40` that is listed both on its own and as part of `192.168.1.0/24`
uses the policy of the single-address client. Each client has at most one
policy. Deleting a client deletes its policy.

To check an address, use the lookup on the Client policies page or
`GET /api/v1/policies/effective?ip=192.168.1.40`.

Policies apply to the address the query arrives from. Devices behind another
resolver or NAT gateway share that gateway's address, so set up the clients
that use this server directly, or give devices fixed DHCP reservations.

## Precedence and performance

Policies replace step 2 of the resolver order:

1. Client access and rate limits
2. The client's filtering policy, or global blocklists and allowlists
3. [Local DNS rewrites](rewrites.md)
4. Local authoritative zones
5. The answer cache
6. [Conditional forwarding](forwarding.md)
7. The global upstream resolvers

Blocked answers are never cached, and cached answers are checked against the
querying client's policy again, so one client's policy never leaks to another
through the cache.

Custom policies are compiled ahead of time; a query only needs the client lookup
and one matcher check. They are rebuilt in the background when blocklists are
added, changed, refreshed or removed. There can be up to 64 policies with up to
1,000 extra domains each.
