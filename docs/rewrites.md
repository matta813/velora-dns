# Local DNS rewrites

Rewrites answer chosen names with fixed data, for example `nas.home -> 192.0.2.5`,
without creating a full authoritative zone. Manage them under **DNS → DNS rewrites**
or with the [`/api/v1/rewrites` endpoints](api.md).

## Record types

| Type | Value | Answer |
|---|---|---|
| `A` | IPv4 address | Answers `A` queries |
| `AAAA` | IPv6 address | Answers `AAAA` queries |
| `CNAME` | Target host name | Answers every query type with the alias; the target is then resolved normally (it may itself be a rewrite, a local zone name or an upstream name) |

A name can carry several `A` and `AAAA` rewrites (up to 16), which are all
returned. A `CNAME` rewrite must be the only rewrite for its name.

Once a name has an address rewrite, queries for other record types (for
example `AAAA` when only `A` is defined, or `MX`) return an empty `NOERROR`
answer. The name is never partly forwarded upstream.

Answers use a TTL of 300 seconds.

## Wildcards

A name like `*.lab.home` matches every name below `lab.home` (`a.lab.home`,
`x.y.lab.home`) but not `lab.home` itself. The wildcard must be the first label
and its parent needs at least two labels, so `*.home` is rejected.

When several rewrites could match, an exact name wins over any wildcard, and the
wildcard with the longest parent wins: for `api.dev.lab.home`, `*.dev.lab.home`
beats `*.lab.home`.

## Precedence

Rewrites are checked after blocklists and before local authoritative zones:

1. Client access and rate limits
2. Blocklists (and allowlists)
3. Local DNS rewrites
4. Local authoritative zones
5. The answer cache
6. Conditional forwarding rules
7. The global upstream resolvers

The rewrite list flags rules that a blocklist answers first ("Blocked") and
rules that replace answers from a local zone. Disabled rewrites are ignored.
Saving or deleting a rewrite removes cached answers for its name (for wildcards,
for the whole parent domain) so the change applies immediately. Rewrites are
stored in the database and survive restarts.
