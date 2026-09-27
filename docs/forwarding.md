# Conditional forwarding

Conditional forwarding sends queries for chosen domains to dedicated resolvers
instead of the global `dns.upstreams`. A typical use is an office or lab domain
served by an internal DNS server, for example `corp.example -> 10.0.0.10`.

Manage rules in the web interface under **DNS → Conditional forwarding** or with
the [`/api/v1/forwarding` endpoints](api.md).

## Matching

- A rule for `corp.example` covers `corp.example` itself and every name below it,
  such as `host.corp.example` and `a.b.corp.example`. Do not enter a wildcard.
- When several rules match, the **most specific** one wins: with rules for
  `corp.example` and `lab.corp.example`, a query for `db.lab.corp.example` uses
  the `lab.corp.example` rule.
- Disabled rules are ignored, so a query falls back to a less specific rule or to
  the global upstreams.
- Names without a matching rule use the global upstreams exactly as before.

## Upstreams

Each rule lists one to four resolvers as `IP` or `IP:port` (port 53 when
omitted). Private addresses are allowed. Hostnames are rejected because resolving
them would itself require DNS. Rules that point at Velora's own DNS listener are
rejected to prevent forwarding loops.

Upstreams are tried in order with the global timeout and retry settings. Each
rule tracks the health of its own upstreams: after repeated failures an upstream
cools down briefly, so a broken internal resolver fails quickly for its domain.
A failing rule never falls back to the global upstreams (which could leak
internal names) and never affects queries for other domains.

DNSSEC validation is not applied to conditionally forwarded answers, because
private zones usually cannot chain to the DNS root.

## Precedence

Queries are answered by the first step that applies:

1. Client access and rate limits
2. Blocklists (and allowlists)
3. [Local DNS rewrites](rewrites.md)
4. Local authoritative zones
5. The answer cache
6. The most specific enabled conditional forwarding rule
7. The global upstream resolvers

Creating, changing or deleting a rule removes cached answers for its domain so
the new route applies immediately.

## Testing a rule

The **Test** action sends one query (default: an `A` query for the rule domain)
through that rule's upstreams only, bypassing the cache and health cooldown. It
works for disabled rules too, so a rule can be verified before it is enabled.
The name must be the rule domain or a name below it.
