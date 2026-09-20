# Velora DNS

**Your DNS. Under your control.** Velora DNS is a self-hosted, forwarding DNS server
written in Go, with a React interface for managing zones, filtering, cache, and
operations. It serves local authoritative answers and forwards other requests to
configured upstream resolvers; it is not an iterative resolver.

[![CI](https://github.com/matta813/velora-dns/actions/workflows/ci.yml/badge.svg)](https://github.com/matta813/velora-dns/actions/workflows/ci.yml)
[![CodeQL](https://github.com/matta813/velora-dns/actions/workflows/codeql.yml/badge.svg)](https://github.com/matta813/velora-dns/actions/workflows/codeql.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> **Project status:** Velora DNS has [beta pre-releases](https://github.com/matta813/velora-dns/releases).
> It is still under active development. The production runtime is single-node;
> multi-node and PostgreSQL cluster code is experimental and not wired into it.
> Review the [production assessment](docs/production-assessment.md) before relying
> on it for critical DNS service.

![Velora DNS dashboard](docs/assets/overview.png)

## What it does

- Serves DNS over UDP, TCP, DoT, DoH, and DoQ, with ordered upstreams, retries,
  client CIDR controls, and bounded concurrency.
- Provides local zones and records, secondary zones via AXFR/IXFR, TSIG, DNSSEC
  validation with configured trust anchors, and an in-memory DNS cache.
- Manages blocklists and allowlists, offers opt-in query history, and exports
  Prometheus metrics without domain or client labels.
- Offers an authenticated management API and responsive UI with admin, operator,
  and viewer roles, scoped API tokens, and SQLite or PostgreSQL management storage.

See the [architecture](docs/architecture/0001-foundation.md),
[DNS behavior](docs/architecture/0002-dns-semantics.md), and
[API reference](docs/api.md) for details and limits.

## Install and try it

The supported installer paths are Docker Compose and a direct systemd installation
on Debian or Ubuntu. Both build from a source checkout and can generate an initial
admin password. Save that password when the installer prints it. The installers ask
for a release channel (`stable`, `beta`, or `alpha`) and whether DNS and the Web UI
should remain local or be exposed to the LAN.

| Mode | Start | Default endpoints |
|---|---|---|
| Docker Compose | `./scripts/install-compose.sh` from a checkout | UI `127.0.0.1:8080`, DNS `127.0.0.1:5353` |
| Direct systemd | `./scripts/install-system-bootstrap.sh` on Debian/Ubuntu | UI `127.0.0.1:8080`, DNS `127.0.0.1:53` |

For a quick local Compose start:

```bash
git clone https://github.com/matta813/velora-dns.git
cd velora-dns
./scripts/install-compose.sh
curl http://127.0.0.1:8080/ready
dig @127.0.0.1 -p 5353 example.org A
```

Open <http://127.0.0.1:8080> and sign in with the bootstrap credentials. The
installer may install Docker and requires `sudo`; review the script before running
it. The [deployment guide](docs/deployment.md) covers manual Compose setup, native
service configuration, LAN access, backups, and upgrades. Keep recursive DNS limited
to trusted client networks.

## Configuration

The server uses strict YAML configuration with explicit `VELORA_*` environment
overrides. A small local example is:

```yaml
dns:
  listen: ['127.0.0.1:5353']
  upstreams: ['1.1.1.1:53', '9.9.9.9:53']
  allowed_clients: ['127.0.0.0/8', '::1/128']
cache:
  max_entries: 10000
  upstream_ttl: 86400
http:
  listen: '127.0.0.1:8080'
  allowed_hosts: ['localhost', '127.0.0.1', '::1']
```

Start the binary with `-config configs/config.yaml`, or use the
[complete example](configs/config.example.yaml). The
[configuration reference](docs/configuration.md) lists defaults, bounds, and
environment variables. Settings in the UI can update supported configuration
values; some changes require a restart.

## Releases and updates

See [GitHub Releases](https://github.com/matta813/velora-dns/releases) for published
pre-releases. Stable, beta, and alpha are the configured update channels; the
[release guide](docs/releases.md) explains channel selection, verification, and
recovery. The [roadmap](docs/roadmap.md) separates current work from exploratory
ideas without promising release dates.

## Develop and contribute

Development requires Go 1.27.1+, Node.js 24+, and npm. From a checkout:

```bash
npm --prefix web ci
make backend   # terminal 1: Go API and DNS server
make frontend  # terminal 2: Vite UI
```

Run `make go-tools` once to install the pinned linter, then `make check` for
formatting, tests, linting, builds, and release-script checks. See
[development](docs/development.md) for package boundaries and test conventions.
Contributions are welcome through focused pull requests; read
[CONTRIBUTING.md](CONTRIBUTING.md) and the [Code of Conduct](CODE_OF_CONDUCT.md).
Report suspected vulnerabilities privately as described in
[SECURITY.md](SECURITY.md); use [GitHub Issues](https://github.com/matta813/velora-dns/issues)
for ordinary bugs and feature requests.

Velora DNS is licensed under the [MIT License](LICENSE). See [NOTICE](NOTICE)
for third-party attribution and project context.
