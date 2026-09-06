# Development

Backend: Go 1.27.1+. Frontend: Node.js 24+, npm lockfile, React, TypeScript strict mode,
Vite, ESLint, Vitest and Testing Library. SQLite uses a pure-Go driver; CGO is not required.

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
- `internal/dns`: transport, access control, resolver and upstream strategies
- `internal/cache`: bounded positive-answer TTL cache
- `internal/metrics`: independent per-instance registry and dashboard counters
- `internal/api`: operational HTTP contract and static web serving
- `web/src`: typed API client, polling, components and pages
- `tests`: local UDP/TCP integration tests

Tests must not depend on the public internet. Use local fake upstreams, ephemeral ports,
controlled clocks where possible, and cancellation. Validate behavior at package boundaries.
Local-zone and filtering tests will land with those implementations, not as placeholder tests.

For a manual end-to-end check, run Compose and query with `dig`. Check HTTP readiness,
cache hit counters, cache flush and SIGTERM exit. Verify desktop/mobile UI layout and
stale snapshot behavior with browser network failures. Documentation screenshots show
the actual running development server, not synthetic dashboard data.
