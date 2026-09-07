# Local authoritative zones

Local zones override forwarded/cache answers immediately after a successful management
mutation. Zone configuration and records persist in SQLite; DNS queries use an immutable
memory snapshot. Missing local names never go to public upstreams. See the
[architecture decision](architecture/0003-local-zones.md) for authoritative semantics.

## Create a zone

```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/zones \
  -H 'Content-Type: application/json' \
  -d '{"name":"home.test"}'
```

The response contains `data` with `id`, normalized `name`, `primary_ns`, `contact`,
`revision` and `records`. `Location` identifies the new zone and `ETag` contains its quoted
revision, initially `"1"`. All updates and deletes require the latest zone revision in
`If-Match`. Requests without it return 428; stale revisions return 412. Reload and review
before retrying a conflict. Duplicate zone names return 409; invalid input returns 400.

```bash
# Substitute the ID returned by zone creation.
curl -i -X POST http://127.0.0.1:8080/api/v1/zones/1/records \
  -H 'Content-Type: application/json' -H 'If-Match: "1"' \
  -d '{"name":"router","type":"A","ttl":300,"value":"192.168.1.1"}'
dig @127.0.0.1 -p 5353 router.home.test A +norecurse
dig @127.0.0.1 -p 5353 home.test SOA
```

`name` accepts `@`, a relative owner (`router`, `deep.branch`), or an absolute name ending
in `.` inside the zone. **A name without its trailing dot is relative**, even if it visually
resembles an FQDN. Target values for CNAME/MX/NS/PTR are absolute names, with the final dot
optional. Names use ASCII labels; enter IDNs as punycode. TXT values are literal strings,
without DNS zone-file quoting. MX uses a separate numeric `priority` (0–65535). TTL is
0–86400 seconds; records in the same RRset must share a TTL.

## Endpoint contract

| Method | Path | Response |
|---|---|---|
| GET | /api/v1/zones | Zone array, including records |
| POST | /api/v1/zones | 201, created zone, Location and ETag |
| GET | /api/v1/zones/:id | Zone and current ETag |
| PUT | /api/v1/zones/:id | Full replacement, updated zone and ETag |
| DELETE | /api/v1/zones/:id | Deleted zone ID |
| GET | /api/v1/zones/:id/records | Record array and zone ETag |
| GET | /api/v1/zones/:id/records/:recordID | Record and zone ETag |
| POST | /api/v1/zones/:id/records | 201, updated **zone**, record Location and new ETag |
| PUT | /api/v1/zones/:id/records/:recordID | Updated zone and new ETag |
| DELETE | /api/v1/zones/:id/records/:recordID | Updated zone and new ETag |

PUT zone replaces the entire record list; omitting `records` removes all explicit records.
Use record endpoints for single-record edits. Zone ID/revision cannot be supplied in zone
payloads. Existing record IDs may be preserved in a full replacement; new records use ID 0
or omit it. Single-record mutations select IDs from the URL and reject supplied IDs.

Deleting a zone also deletes its records. No automatic PTR records are created: create an
appropriate `in-addr.arpa` or `ip6.arpa` zone and an explicit PTR record. A longest-suffix
match selects nested zones; parent records hidden by a child zone do not answer its queries.

SOA and fallback apex NS are generated. Defaults are `ns.<zone>` and `hostmaster.<zone>`
(the DNS mailbox form), and can be changed via PUT. Add A/AAAA records for the chosen
nameserver if needed. Wildcards, delegation, CNAME at apex, duplicate records, conflicting
CNAME data and local alias loops are rejected. Limits are 256 zones, 1000 records per zone
and 10000 records total. Existing client-side DNS caches retain their advertised TTLs.

## Operations

Migration from the foundation creates zone tables transactionally at startup. Back up the
volume before upgrading. An older binary cannot manage the new zone state; keep the backup
for rollback. Never modify live SQLite tables manually: management changes publish snapshots
and invalidate forwarding cache, whereas out-of-band SQL cannot notify the running resolver.
Management remains unauthenticated and must stay on trusted interfaces.
