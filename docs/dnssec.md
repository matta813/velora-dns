# DNSSEC validation

Velora can validate recursive upstream answers locally. It requests DNSSEC records from
the selected upstream, builds the DS/DNSKEY chain to a configured trust anchor, verifies
RRsets and signature lifetimes, and checks NSEC or NSEC3 proofs for NXDOMAIN and NODATA.
A validated answer carries AD. An invalid (bogus) answer is rejected and results in
SERVFAIL after upstream failover is exhausted. An authenticated insecure delegation is
returned without AD.

Validation is deliberately opt-in because trust anchors are operational security data:

```yaml
dns:
  dnssec: true
  trust_anchors:
    - '. 3600 IN DS <key-tag> <algorithm> <digest-type> <digest>'
```

Each entry must be a complete DS record. Obtain root anchors from IANA's official root
trust-anchor publication and verify that source out of band. Velora does not ship a
hard-coded anchor that can silently age. Configure both the retiring and replacement DS
during a rollover; multiple anchors for the same owner are accepted. Remove the old DS
only after the rollover has completed. Configuration is reloaded on process restart.

Velora always sends DO upstream while validation is enabled. A client does not need to
set DO to receive validation; DO only controls whether DNSSEC records are included in the
reply. A client setting CD requests unchecked data, so Velora skips validation and clears
AD. Locally served zones and filtering responses are not signed and never carry AD.

Operators should monitor SERVFAIL after anchor changes and keep system time synchronized,
because signature inception and expiration are enforced. Automated tests use generated
keys and local upstreams and therefore require no public DNS access.
