# Development

Backend: Go 1.27.1+. Frontend: Node.js 24+, npm lockfile, React, TypeScript strict mode,
Vite, ESLint, Vitest and Testing Library. SQLite uses a pure-Go driver; CGO is not required.
PostgreSQL support uses `jackc/pgx/v5`.

```bash
npm --prefix web ci
make go-tools
make check
```

`make check` includes format, vet, static analysis, race tests, frontend lint/types/tests,
production builds and release-automation tests. Docker Compose is needed for its static
configuration check. The analyzer is pinned to v2.13.2 for Go 1.27 support.

Run `make backend` and `make frontend` in separate terminals for hot reload. Production
serves `web/dist` from the Go HTTP process. No Node server runs in the final image.

Package boundaries:

- `cmd/server`: flags, process signals, build identity
- `internal/app`: composition and lifecycle
- `internal/config`, `logging`, `database`: typed config, JSON logs and management persistence
- `internal/dns`: transport (UDP/TCP/DoT/DoH/DoQ), access control, resolver, upstream strategies, TSIG and zone transfers
- `internal/cache`: bounded positive-answer TTL cache
- `internal/zones`: record validation, immutable authority snapshots, revision-protected mutations and secondary zone management
- `internal/metrics`: independent per-instance registry and dashboard counters
- `internal/api`: operational HTTP contract, static web serving, TSIG/secondary zone endpoints
- `internal/node`: node membership, health monitoring and capability negotiation
- `internal/replication`: experimental, non-runtime scaffolding for future multi-node management
- `web/src`: typed API client, polling, components and pages
- `tests`: local UDP/TCP integration tests

Run `make test-e2e` to start a fresh Velora instance with a temporary SQLite
database and local listeners. The test signs in, creates and updates a zone
record through the management API, verifies real DNS answers, checks query
history and validation errors, then confirms a viewer cannot mutate zones.
It uses only loopback addresses and runs as part of the existing backend CI
test job. Failure output includes the API response and application logs.

Tests must not depend on the public internet. Use local fake upstreams, ephemeral ports,
controlled clocks where possible, and cancellation. Validate behavior at package boundaries.
Local-zone tests cover authority boundaries, record matching, CNAME resolution, transactional
persistence and concurrent edits. TSIG tests verify HMAC signing and verification. Transfer
tests use mock primary servers.

For a manual end-to-end check, run Compose and query with `dig`. Check HTTP readiness,
cache hit counters, cache flush and SIGTERM exit. Verify desktop/mobile UI layout and
stale snapshot behavior with browser network failures. Documentation screenshots show
the actual running development server, not synthetic dashboard data.

## Accessibility and responsive checks

`web/scripts/a11y-audit.mjs` loads every page with the shared demo responses from
`web/scripts/fixtures.mjs` at phone (390 px), tablet (820 px) and desktop (1440 px)
widths in light and dark themes, and fails on:

- axe-core violations of the WCAG 2.2 A/AA rules (contrast, names, roles, landmarks)
- horizontal page overflow (tables scroll inside their own region instead)
- controls smaller than the 24 × 24 px target size (WCAG 2.5.8); a checkbox counts
  its label as the target
- keyboard focus that is not visibly indicated while tabbing

```bash
cd web
npm run build
npx vite preview --port 4173 &
npm run audit:a11y -- --base-url http://127.0.0.1:4173
```

Pass `--pages /zones` (repeatable) or `--theme dark` to narrow a run and set
`CHROMIUM_PATH` to use an existing Chromium. Unit tests cover keyboard behavior that
the audit cannot see: the navigation dropdowns follow the menu-button pattern (arrow
keys, Home/End, Escape returns focus), the mobile menu traps focus and closes with
Escape, focus moves to the page heading after navigation, scrollable tables become
labelled focusable regions only when they overflow, and the document language
follows the selected UI language. Animations and transitions are disabled when the
system asks for reduced motion.
