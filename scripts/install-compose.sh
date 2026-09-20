#!/usr/bin/env sh
# Install Docker Engine with the Compose plugin, then start Velora DNS.
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
export VELORA_CHANNEL="$channel" VELORA_HTTP_HOST="$http_host" VELORA_DNS_HOST="$dns_host"

command -v sudo >/dev/null 2>&1 || die "sudo is required to install Docker"
[ -r /etc/os-release ] || die "only Debian and Ubuntu are supported by this installer"
. /etc/os-release
case "${ID:-}" in
  debian|ubuntu) ;;
  *) die "only Debian and Ubuntu are supported by this installer" ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ -f "$script_dir/../docker-compose.yml" ]; then
  repo_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
else
  # The script may be run directly from GitHub with curl. Fetch a checkout so
  # Compose has the Dockerfile, configuration, and application sources to build.
  repo_dir=${VELORA_INSTALL_DIR:-/opt/velora/compose}
  if [ ! -e "$repo_dir" ]; then
    printf '%s\n' "Downloading Velora DNS into $repo_dir…"
    sudo apt-get update
    sudo apt-get install -y git
    sudo git clone --depth 1 https://github.com/matta813/velora-dns.git "$repo_dir"
  fi
  [ -f "$repo_dir/docker-compose.yml" ] || die "$repo_dir is not a Velora DNS checkout; set VELORA_INSTALL_DIR to an empty directory or a checkout"
fi
cd "$repo_dir"

if ! docker compose version >/dev/null 2>&1; then
  command -v curl >/dev/null 2>&1 || die "curl is required to install Docker"
  command -v gpg >/dev/null 2>&1 || die "gpg is required to install Docker"
  printf '%s\n' 'Installing Docker Engine and the Docker Compose plugin from Docker’s official APT repository…'
  sudo apt-get update
  sudo apt-get install -y ca-certificates curl gnupg
  sudo install -d -m 0755 /etc/apt/keyrings
  curl -fsSL "https://download.docker.com/linux/$ID/gpg" | sudo gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
  sudo chmod a+r /etc/apt/keyrings/docker.gpg
  codename=${VERSION_CODENAME:-}
  [ -n "$codename" ] || die "could not determine the distribution codename"
  printf '%s\n' "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$ID $codename stable" | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
  sudo apt-get update
  sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
fi

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

sudo install -d -m 0755 /etc/velora
if ! sudo test -f /etc/velora/updater.env; then
  printf 'VELORA_CHANNEL=%s\n' "$channel" | sudo tee /etc/velora/updater.env >/dev/null
  sudo chmod 0644 /etc/velora/updater.env
fi

sudo install -d -m 0700 /run/velora
compose_env=$(sudo mktemp /run/velora/.env.XXXXXX)
sudo chmod 0600 "$compose_env"
printf 'VELORA_BOOTSTRAP_USERNAME=%s\nVELORA_BOOTSTRAP_PASSWORD=%s\nVELORA_CHANNEL=%s\nVELORA_HTTP_HOST=%s\nVELORA_DNS_HOST=%s\n' "$bootstrap_user" "$bootstrap_password" "$channel" "$http_host" "$dns_host" | sudo tee "$compose_env" >/dev/null
sudo docker compose --env-file "$compose_env" up --build -d
sudo rm -f "$compose_env"
sudo docker compose ps

if ! id -nG "$USER" | tr ' ' '\n' | grep -qx docker; then
  sudo usermod -aG docker "$USER"
  printf '%s\n' "Added $USER to the docker group; sign out and back in before using Docker without sudo."
fi
printf '%s\n' 'Velora DNS is running. Open http://127.0.0.1:8080'
if [ "$generated_password" = true ]; then
  printf '%s\n' "Bootstrap username: $bootstrap_user"
  printf '%s\n' "Bootstrap password: $bootstrap_password"
  printf '%s\n' 'Store this password now; it is passed only to the initial container start.'
fi
