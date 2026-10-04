# Development validation

Validated locally on 2026-09-07 with Go 1.27.1 and Docker Engine 29.8.0.
The project is a tested development foundation, not a production certification.

Partly re-checked on 2026-10-04 (Go 1.27.1, Docker 29.8.1): `go test ./internal/dns ./tests`
passed, `python3 scripts/check-markdown-links.py` passed, and the Makefile, CI
workflows and test names were compared against this page. The results table below
is **not** a fresh run of every check: its rows keep their 2026-09-07 evidence.
Not re-run: gofmt, go vet, golangci-lint, race tests, fuzzing, the frontend suite
(`node_modules` was absent), the browser suite, Compose and live DNS checks.

| Area | Evidence |
|---|---|
| Backend | gofmt, go vet, golangci-lint v2.13.2 and race tests pass |
| DNS | Local ephemeral UDP/TCP integration tests for A, AAAA, CNAME, TXT, MX, NS and PTR |
| Forwarding | Failover, retries, context cancellation, question validation, CNAME-chain validation, EDNS option privacy and large-answer TCP fallback |
| Protocol hardening | RFC 2308 NXDOMAIN/NODATA TTL boundaries, malformed EDNS, DNS Cookie challenge/replay behavior and parser policy fuzz seeds |
| Cache | TTL aging, expiry sweep, exact expiration, copy isolation, LRU capacity and flush |
| Config | Strict YAML, environment overrides, bounds, host validation; over 58,000 fuzz executions without failure |
| HTTP | Database/listener readiness, origin/Host protection, request limits and JSON method errors |
| Local zones | Matching, SOA/NXDOMAIN/NODATA, CNAME chains/loops, nested zones, atomic snapshots, migration/reopen and optimistic revision tests |
| Zone API | CRUD, ETag/If-Match, stale writers, record ownership, strict JSON and chunked payload limits |
| Frontend | TypeScript, ESLint, Vitest/Testing Library and Vite production build pass |
| Go workflows | `make test-e2e`: management/DNS, statistics restart, backup/restore, updater, rate limit and two-process cluster replication against real listeners (see [development](development.md#end-to-end-tests)) |
| Browser | Real desktop/mobile navigation, cache flush, settings/zone route reload, zone/record CRUD, conflict handling and no horizontal mobile overflow |
| Container | Compose build/start, healthy readiness, UID/GID 10001, read-only root, persistent private SQLite files after restart |
| Live DNS | google.com A over UDP, AAAA over TCP, repeated A query served from cache |
| Metrics | All required metric families observed; no domain or client-IP labels |
| Release infrastructure | SemVer, immutable recovery, release-note classification and workflow invariants tested without publication |

Requests using ordinary `dig` cookies intentionally bypass shared cache. Use `+nocookie`
when demonstrating cache reuse. Public upstream smoke tests require network egress;
automated DNS tests use local upstreams only.

The CI workflow repeats backend/frontend checks, runs the Playwright and accessibility suites
against a real server, and builds the container without pushing it. CodeQL, dependency review and Go vulnerability scanning are separate required checks.
Repository branch protection and release-disable state were checked through the GitHub API.

Query history now has real SQLite round-trip, row/age retention, JSON contract,
queue-overflow, shutdown-drain and DNS-source integration tests. Browser checks use
an isolated database with logging enabled and exercise actual DNS responses, filters,
direct navigation and mobile layout. Remaining blocklist work is tracked in roadmap issues.
Local zone browser checks
also verified real UDP/TCP answers, immediate changes and persistence across a Compose
restart. The temporary test zone was removed; screenshots show that test fixture. Do not infer implementation
from planned package names or future API descriptions.
