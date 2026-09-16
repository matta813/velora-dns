# ADR 0004: High availability and failure semantics

## Status

Accepted as the prerequisite contract for issues #29–#32. It does not make the
current single-node service highly available.

## Context

Authoritative zone changes and configuration changes must not be silently lost or
diverge across DNS nodes. A network partition cannot be distinguished reliably
from a failed peer, so availability during a partition must be chosen explicitly.

## Decision

Velora will use a quorum-based control plane with a single elected leader for each
write epoch. Membership is authenticated (#29). Zone and configuration mutations
are assigned a monotonically increasing revision and become externally successful
only after durable acknowledgement by a majority of voting nodes (#30, #31).

DNS query serving is a data-plane concern: a node may serve its last committed,
locally durable zone snapshot while disconnected. It must not accept management
writes, originate transfers, or claim healthy control-plane membership without a
leader quorum. Recursive forwarding remains node-local and its cache is explicitly
outside replicated state.

| Situation | Reads | Writes | Health-routing state |
|---|---|---|---|
| Leader has quorum | Serve committed snapshot | Accept after quorum durability | Ready |
| Follower has leader/quorum | Serve committed snapshot | Redirect/reject as not leader | Ready |
| Partitioned minority | Serve last committed authoritative data only | Reject | Degraded; remove from write-capable pool |
| No quorum | Serve last committed authoritative data only | Reject | Degraded |
| Node restart/rejoin | Serve only after local recovery | Catch up before participating | Not ready until caught up |

This favors consistency for management writes over write availability. It prevents
two partitions from independently committing a revision. A stale authoritative
answer is preferable to manufacturing inconsistent zone data, but its age and
degraded state must be observable.

## Failure domains and routing

Place voters across independent failure domains (host, rack/AZ, and power/network
where available). Use an odd number of voters: three tolerates one loss; five
tolerates two. Do not count a passive DNS-only replica as a quorum voter. Health
checks must separately expose listener reachability, local snapshot freshness,
control-plane quorum, leader reachability, replication lag, and storage health.

Load balancers may route DNS reads to nodes with a valid committed snapshot. They
must route management writes only to the elected leader or a proxy that verifies
leader term and quorum. A node reporting degraded must never be selected for a
write path merely because its TCP port is open.

## Recovery and chaos plan

Before enabling multi-node mode, automated integration tests must demonstrate:

1. leader loss and election without two committed revisions for one predecessor;
2. minority partition rejects writes while retaining only its committed snapshot;
3. healed minority catches up atomically before it becomes ready;
4. delayed, replayed and duplicated replication messages cannot roll back a newer
   revision or cross an epoch boundary;
5. disk-full, crash during persistence, and corrupt journal recovery preserve the
   last durable revision and require operator-visible remediation;
6. rolling upgrades across a mixed-version cluster negotiate capabilities and do
   not enable an unsupported protocol or migration;
7. load balancer health routing excludes no-quorum nodes from writes and records
   a bounded degraded metric without node/client high-cardinality labels.

Each scenario must assert both data equality and the exposed health state, then
run under packet loss, latency, reordering and complete partitions. Manual chaos
drills must retain logs, revision histories and recovery timing.

## Consequences

The current application remains single-node until the required authenticated
membership, durable log/revision protocol, recovery path and tests exist. Future
issues must not introduce best-effort multi-writer synchronization or hide a
partial replication failure behind a generic healthy status.
