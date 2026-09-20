# Contributing to Velora DNS

Thanks for helping improve Velora DNS. For substantial changes, open an
[issue](https://github.com/matta813/velora-dns/issues) first to discuss scope.
Small fixes and documentation corrections can go straight to a PR. Follow the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Set up a checkout

You need Go 1.27.1+, Node.js 24+, and npm. Docker Compose is useful for manual
deployment checks. From the repository root:

```bash
npm --prefix web ci
make go-tools
make backend   # terminal 1
make frontend  # terminal 2
```

The [development guide](docs/development.md) describes package boundaries and
test conventions. Tests must use local fake upstreams, not public DNS. Preserve
context cancellation and bounded resource use in server code. Avoid placeholder
API responses that suggest an operation succeeded.

## Prepare a pull request

1. Create a focused branch from `main`. Include tests and update documentation for
   changed behavior. Keep English and German UI strings aligned when changing text.
2. Run `make check` (or explain any unavailable check in the PR). For docs-only
   changes, run `python3 scripts/check-markdown-links.py` at minimum.
3. Use a [Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/)
   title such as `fix(dns): preserve upstream timeout context`.
4. Describe the problem, solution, verification, and configuration or migration
   impact in the PR. Link the issue it resolves.

Do not commit directly to `main`. Never include passwords, tokens, private DNS query
data, or other secrets in issues, commits, screenshots, or test fixtures.

Report suspected vulnerabilities through [SECURITY.md](SECURITY.md), not a public issue.
