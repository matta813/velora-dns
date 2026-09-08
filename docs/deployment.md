# Deployment

## Local Docker start

```bash
VELORA_BOOTSTRAP_PASSWORD='use-a-unique-password-manager-value' docker compose up --build -d
docker compose ps
curl http://127.0.0.1:8080/ready
dig @127.0.0.1 -p 5353 google.com A
```

No published project image is required. If another service uses port 8080:

```bash
VELORA_HTTP_PORT=18080 docker compose up --build -d
```

The container runs as UID/GID 10001, has a read-only root filesystem, no Linux capabilities,
no privilege escalation, bounded processes, CPU, memory and rotated logs. SQLite is stored
in `velora-data:/data`; the root-owned host bind mount alternative must grant UID 10001
access. A built-in HTTP client runs the readiness healthcheck. `docker compose down`
retains data; do not add `--volumes` unless you intend to erase it.

## LAN and standard DNS ports

The application defaults to port 5353 to support unprivileged development. Standard DNS
uses both UDP and TCP 53. In Docker, publish a specific LAN interface on host port 53 to
container port 5353; no privileged container is needed:

```yaml
# Edit the ports and environment of the existing Compose service.
ports:
  - '192.168.1.10:53:5353/udp'
  - '192.168.1.10:53:5353/tcp'
  - '127.0.0.1:8080:8080/tcp'
environment:
  VELORA_DNS_ALLOWED_CLIENTS: '192.168.1.0/24,172.18.0.1/32'
```

Replace both addresses with your actual LAN and Docker gateway. Inspect the Compose
network to determine gateway addressing. Docker NAT can replace host-local client source
addresses; the sample's private-range defaults support that development case. Other
containers on the bridge may also reach the container, so narrow CIDRs for a real deployment.
Add the exact management hostname to `VELORA_HTTP_ALLOWED_HOSTS` when using a reverse proxy.
Do not publish management ports to untrusted networks. Restrict forwarding in the host
firewall; Docker forwarding can behave differently from host INPUT firewall rules.

For bare-metal port 53, bind a dedicated service to `0.0.0.0:53` and/or `[::]:53` and grant
only `CAP_NET_BIND_SERVICE` through your service manager. Do not run the whole application
as root. Configure actual client CIDRs. Native listeners support IPv6; example Compose
publishes IPv4 loopback only. Check for system DNS services already occupying port 53.

## Upgrades and backups

For this unreleased source-build phase, stop the service, record the Git commit, back up
its data, check out the reviewed main branch and rebuild. Only schema metadata is currently
persisted; future migrations will document compatibility and rollback restrictions.

For a consistent offline backup, stop the container and copy the complete named volume
using your normal volume backup tooling. For an online backup, use SQLite's backup API;
do not copy only a live database file while ignoring its WAL. Protect filesystem access.
Shutdown drains HTTP and DNS and checkpoints WAL. Verify backups by restoring into an
isolated volume and starting the same source version.

## Operational checks

`/health` means the process serves HTTP. `/ready` checks live SQLite access and initialized
DNS listeners. Upstream availability is deliberately not a readiness dependency: temporary
internet loss must not create a restart loop. Inspect upstream error metrics for it.
Metrics and counters reset at process restart. Cache is intentionally ephemeral.
