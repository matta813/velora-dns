# Contributing

Open an issue to discuss larger changes. Use feature branches and small pull requests;
do not commit features directly to main. Use Conventional Commits, for example
`feat(dns): add upstream failover`. Include behavior, tests, and documentation.

Run `make check` before opening a PR. Tests must use local fake upstreams, not public
DNS. Preserve context cancellation, bounded resource use, and package boundaries.
Do not add placeholder endpoints that return fabricated success.

Report vulnerabilities privately as described in SECURITY.md.
