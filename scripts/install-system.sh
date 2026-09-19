#!/usr/bin/env sh
# Install Velora DNS from this source checkout as a hardened systemd service.
set -eu

die() {
  printf '%s\n' "error: $*" >&2
  exit 1
}

prompt_choice() {
  prompt=$1
  default=$2
  answer=
  if [ -e /dev/tty ]; then
    printf '%s' "$prompt" > /dev/tty 2>/dev/null || true
    read -r answer < /dev/tty 2>/dev/null || true
  fi
  printf '%s\n' "${answer:-$default}"
}

command -v go >/dev/null 2>&1 || die "Go 1.27.1+ is required (https://go.dev/dl/)"
command -v npm >/dev/null 2>&1 || die "Node.js 24+ and npm are required (https://nodejs.org/)"
command -v systemctl >/dev/null 2>&1 || die "this installer requires a systemd-based Linux host"
command -v sudo >/dev/null 2>&1 || die "sudo is required to install the service"

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_dir"

channel=${VELORA_CHANNEL:-}
if [ -z "$channel" ]; then
  channel=$(prompt_choice 'Release channel [stable/beta/alpha] (stable): ' stable)
fi
case "$channel" in
  stable|beta|alpha) ;;
  *) die 'VELORA_CHANNEL must be stable, beta, or alpha' ;;
esac

http_host=${VELORA_HTTP_HOST:-}
if [ -z "$http_host" ]; then
  expose_web=$(prompt_choice 'Expose the Web UI on all network interfaces? [y/N]: ' N)
  case "$expose_web" in
    y|Y|yes|YES) http_host=0.0.0.0; printf '%s\n' 'Web UI will be reachable on every interface; restrict network access with a firewall.' ;;
    *) http_host=127.0.0.1 ;;
  esac
fi
case "$http_host" in
  127.0.0.1|0.0.0.0) ;;
  *) die 'VELORA_HTTP_HOST must be 127.0.0.1 or 0.0.0.0' ;;
esac

dns_host=${VELORA_DNS_HOST:-}
if [ -z "$dns_host" ]; then
  expose_dns=$(prompt_choice 'Expose DNS to your local network? [y/N]: ' N)
  case "$expose_dns" in
    y|Y|yes|YES) dns_host=0.0.0.0; printf '%s\n' 'DNS will accept private-network clients; confirm your firewall permits only trusted clients.' ;;
    *) dns_host=127.0.0.1 ;;
  esac
fi
case "$dns_host" in
  127.0.0.1|0.0.0.0) ;;
  *) die 'VELORA_DNS_HOST must be 127.0.0.1 or 0.0.0.0' ;;
esac

go_version=$(go env GOVERSION | sed 's/^go//')
if [ "$(printf '%s\n%s\n' '1.27.1' "$go_version" | sort -V | head -n 1)" != '1.27.1' ]; then
  die "Go 1.27.1+ is required; found $go_version"
fi

printf '%s\n' 'Building the Velora DNS binary and dashboard…'
npm --prefix web ci
npm --prefix web run build
CGO_ENABLED=0 go build -trimpath -o bin/velora-dns ./cmd/server

bootstrap_user=${VELORA_BOOTSTRAP_USERNAME:-admin}
bootstrap_password=${VELORA_BOOTSTRAP_PASSWORD:-}
if [ -z "$bootstrap_password" ]; then
  command -v openssl >/dev/null 2>&1 || die "openssl is required to generate a bootstrap password (or set VELORA_BOOTSTRAP_PASSWORD)"
  bootstrap_password=$(openssl rand -base64 24)
  generated_password=true
else
  generated_password=false
fi
[ "${#bootstrap_password}" -ge 12 ] || die "VELORA_BOOTSTRAP_PASSWORD must be at least 12 characters"
case "$bootstrap_user:$bootstrap_password" in
  *[!A-Za-z0-9._+/@=:-]*) die "bootstrap credentials may contain only letters, numbers, . _ + / @ = : and -" ;;
esac

sudo install -d -o root -g root -m 0755 /opt/velora /etc/velora
if ! getent group velora >/dev/null; then
  sudo groupadd --system velora
fi
if ! id -u velora >/dev/null 2>&1; then
  sudo useradd --system --home-dir /var/lib/velora --shell /usr/sbin/nologin --gid velora velora
fi
sudo install -d -o velora -g velora -m 0750 /var/lib/velora
sudo install -o root -g root -m 0755 bin/velora-dns /opt/velora/velora-dns
sudo rm -rf /opt/velora/web
sudo install -d -o root -g root -m 0755 /opt/velora/web
sudo cp -R web/dist /opt/velora/web/dist
sudo chown -R root:root /opt/velora/web

if ! sudo test -f /etc/velora/config.yaml; then
  if [ "$http_host" = 0.0.0.0 ]; then
    http_allowed_hosts="['*']"
  else
    http_allowed_hosts="['localhost', '127.0.0.1', '::1']"
  fi
  sudo tee /etc/velora/config.yaml >/dev/null <<EOF
# Standard DNS installation. Restrict listener and client CIDRs before untrusted-network use.
dns:
  listen: ['$dns_host:53']
  upstreams: ['1.1.1.1:53', '9.9.9.9:53']
  allowed_clients: ['127.0.0.0/8', '::1/128']
  timeout: 2s
  retries: 1
  max_concurrent: 256
  global_qps: 1000
  client_qps: 100
  rate_limit_burst: 100
  max_tcp_connections: 256
cache:
  max_entries: 10000
http:
  listen: '$http_host:8080'
  web_dir: /opt/velora/web/dist
  allowed_hosts: $http_allowed_hosts
database_path: /var/lib/velora/velora.db
log_level: info
EOF
  sudo chmod 0640 /etc/velora/config.yaml
fi

if ! sudo test -f /etc/velora/updater.env; then
  printf 'VELORA_CHANNEL=%s\n' "$channel" | sudo tee /etc/velora/updater.env >/dev/null
  sudo chmod 0644 /etc/velora/updater.env
fi

if ! sudo test -f /etc/velora/velora.env; then
  sudo tee /etc/velora/velora.env >/dev/null <<EOF
VELORA_BOOTSTRAP_USERNAME=$bootstrap_user
VELORA_BOOTSTRAP_PASSWORD=$bootstrap_password
EOF
  sudo chown root:velora /etc/velora/velora.env
  sudo chmod 0640 /etc/velora/velora.env
fi

sudo tee /etc/systemd/system/velora-dns.service >/dev/null <<'EOF'
[Unit]
Description=Velora DNS
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=velora
Group=velora
EnvironmentFile=/etc/velora/velora.env
ExecStart=/opt/velora/velora-dns -config /etc/velora/config.yaml
Restart=on-failure
RestartSec=5s
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/velora

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now velora-dns
sudo systemctl --no-pager --full status velora-dns

printf '%s\n' 'Velora DNS is running. Open http://127.0.0.1:8080'
if [ "$generated_password" = true ]; then
  printf '%s\n' "Bootstrap username: $bootstrap_user"
  printf '%s\n' "Bootstrap password: $bootstrap_password"
  printf '%s\n' 'Store this password now; it is saved in /etc/velora/velora.env for the first start only.'
fi
