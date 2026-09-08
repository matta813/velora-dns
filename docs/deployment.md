# Deployment

## Local Docker start

```bash
docker compose up --build -d
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

### Tested native dual-stack example

[`configs/config.lan.example.yaml`](../configs/config.lan.example.yaml) is parsed by the
Go configuration tests. It listens for DNS on IPv4 and IPv6 port 53, permits one example
IPv4 `/24` and IPv6 `/64`, and keeps management on IPv4 loopback. Replace
`192.168.50.0/24` and `fd12:3456:789a::/64` with routed prefixes you control.

Run without root by granting only the bind capability to the built binary:

```ini
[Service]
User=velora
Group=velora
ExecStart=/opt/velora/velora-dns -config /etc/velora/config.yaml
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/var/lib/velora
```

Set `database_path: /var/lib/velora/velora.db` for that unit. Reach the loopback-only UI
remotely with `ssh -L 8080:127.0.0.1:8080 dns-host`.

Open DNS only from real client prefixes. Choose the firewall installed on the host and
preserve its established/loopback rules:

```bash
# nftables (inet family covers IPv4 and IPv6)
nft add rule inet filter input ip saddr 192.168.50.0/24 udp dport 53 accept
nft add rule inet filter input ip saddr 192.168.50.0/24 tcp dport 53 accept
nft add rule inet filter input ip6 saddr fd12:3456:789a::/64 udp dport 53 accept
nft add rule inet filter input ip6 saddr fd12:3456:789a::/64 tcp dport 53 accept

# firewalld (make permanent after testing)
firewall-cmd --add-rich-rule='rule family=ipv4 source address=192.168.50.0/24 port port=53 protocol=udp accept'
firewall-cmd --add-rich-rule='rule family=ipv4 source address=192.168.50.0/24 port port=53 protocol=tcp accept'
firewall-cmd --add-rich-rule='rule family=ipv6 source address=fd12:3456:789a::/64 port port=53 protocol=udp accept'
firewall-cmd --add-rich-rule='rule family=ipv6 source address=fd12:3456:789a::/64 port port=53 protocol=tcp accept'

# UFW
ufw allow from 192.168.50.0/24 to any port 53 proto udp
ufw allow from 192.168.50.0/24 to any port 53 proto tcp
ufw allow from fd12:3456:789a::/64 to any port 53 proto udp
ufw allow from fd12:3456:789a::/64 to any port 53 proto tcp
```

Do not add a LAN rule for port 8080 with this pattern. Verify from allowed IPv4 and IPv6
clients with `dig`; a host outside both CIDRs should receive REFUSED if it reaches Velora.

### Tested Compose dual-stack example

[`configs/compose.lan.example.yaml`](../configs/compose.lan.example.yaml) is validated by
`scripts/test-compose.sh` with the base Compose file. It replaces loopback DNS publication
with explicit IPv4 and IPv6 host addresses on UDP/TCP 53 while leaving management on
`127.0.0.1`. Docker must have IPv6 enabled and both addresses must belong to the host.
Replace the examples, validate the merged model, then start it:

```bash
docker compose -f docker-compose.yml -f configs/compose.lan.example.yaml config --quiet
docker compose -f docker-compose.yml -f configs/compose.lan.example.yaml up --build -d
```

Published ports may traverse Docker forwarding/NAT rather than the host INPUT chain.
Enforce source prefixes in both `VELORA_DNS_ALLOWED_CLIENTS` and the Docker firewall path
(`DOCKER-USER` on iptables-based hosts, or the platform-equivalent nftables chain). The
example permits `172.16.0.0/12` because Docker may present a bridge source; narrow it to
the observed gateway `/32` or `/128` after `docker network inspect`. Never publish port
8080 on `0.0.0.0` or `[::]` merely to make management convenient.

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
