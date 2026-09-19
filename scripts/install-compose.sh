#!/usr/bin/env sh
# Install Docker Engine with the Compose plugin, then start this checkout.
set -eu

die() {
  printf '%s\n' "error: $*" >&2
  exit 1
}

command -v sudo >/dev/null 2>&1 || die "sudo is required to install Docker"
[ -r /etc/os-release ] || die "only Debian and Ubuntu are supported by this installer"
. /etc/os-release
case "${ID:-}" in
  debian|ubuntu) ;;
  *) die "only Debian and Ubuntu are supported by this installer" ;;
esac

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
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

sudo env VELORA_BOOTSTRAP_USERNAME="$bootstrap_user" VELORA_BOOTSTRAP_PASSWORD="$bootstrap_password" docker compose up --build -d
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
