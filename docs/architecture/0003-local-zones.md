# ADR 0003: Authoritative local zones

Status: accepted.

Local zones precede cache and forwarding. The closest enclosing configured zone owns the
question; a local NXDOMAIN or NODATA response is final and never leaks upstream. Empty
non-terminals exist when descendants exist. A/AAAA/CNAME/TXT/MX/NS/PTR records are supported;
SOA is generated from the zone revision and metadata. Apex NS falls back to the configured
primary nameserver if no explicit apex NS records exist. Operators must create matching
address records if they intend other resolvers to reach that nameserver by name.

Names normalize to lowercase absolute ASCII names. Owners accept `@`, relative labels,
or absolute names within the zone. Targets are absolute domain names, with the trailing
dot optional. Wildcards, delegation NS below the apex and CNAME at the apex are rejected.
TXT values are literal UTF-8 strings, split into DNS character strings without altering
content. RRsets require uniform TTLs (0–86400 seconds). Duplicate records, CNAME coexistence,
local CNAME loops and chains exceeding 16 names are rejected before persistence.

Every mutation validates the complete candidate snapshot, commits a whole-zone SQLite
transaction, then atomically swaps the immutable in-memory view and flushes the forwarding
cache. DNS reads do not execute SQL or lock management writes. In-flight queries may finish
using the previous snapshot; new queries see the published state. Failed persistence never
publishes a candidate. Zone revisions provide optimistic concurrency and the SOA serial;
record IDs remain stable across zone revisions. A maximum of 256 zones, 1000 records per
zone and 10000 total records bounds management memory and payload processing.

Local CNAME chains follow the closest configured zone. Recursive requests complete an
external terminal target through the existing resolver; nonrecursive requests return the
local CNAME chain. Local data is not inserted into the upstream TTL cache. DNSSEC validation,
secondary zones, delegation, wildcard synthesis and transfers remain future work.

SOA-backed negative answers use a 60-second negative TTL, following
[RFC 2308](https://www.rfc-editor.org/rfc/rfc2308.html). Resolution and alias handling follow
[RFC 1034](https://www.rfc-editor.org/rfc/rfc1034.html). Existing clients may cache answers
until their advertised TTL expires even when the server's state changes immediately.

The storage interface belongs to `zones`; the SQLite adapter lives in `database`. A later
PostgreSQL adapter can implement the same transaction and revision contract. Schema upgrade
from the foundation is tested, with ordered migrations applied once inside a transaction.
