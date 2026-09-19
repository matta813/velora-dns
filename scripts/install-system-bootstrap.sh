#!/usr/bin/env sh
# Bootstrap a source checkout and build dependencies for the systemd installer.
set -eu

die() {
  printf '%s\n' "error: $*" >&2
  exit 1
}

command -v sudo >/dev/null 2>&1 || die "sudo is required to install Velora DNS"
command -v systemctl >/dev/null 2>&1 || die "this installer requires a systemd-based Linux host"
[ -r /etc/os-release ] || die "only Debian and Ubuntu are supported by this installer"
. /etc/os-release
case "${ID:-}" in
  debian|ubuntu) ;;
  *) die "only Debian and Ubuntu are supported by this installer" ;;
esac

case "$(uname -m)" in
  x86_64) platform_arch=amd64; node_arch=x64 ;;
  aarch64|arm64) platform_arch=arm64; node_arch=arm64 ;;
  *) die "only amd64 and arm64 are supported by this installer" ;;
esac

repo_dir=${VELORA_INSTALL_DIR:-/opt/velora/source}
if [ ! -e "$repo_dir" ]; then
  printf '%s\n' "Downloading Velora DNS into $repo_dir…"
  sudo apt-get update
  sudo apt-get install -y ca-certificates curl git xz-utils
  sudo git clone --depth 1 https://github.com/matta813/velora-dns.git "$repo_dir"
elif [ -d "$repo_dir/.git" ]; then
  printf '%s\n' "Updating the Velora DNS checkout in $repo_dir…"
  sudo git -C "$repo_dir" fetch origin --depth 1
  sudo git -C "$repo_dir" reset --hard origin/main
  sudo git -C "$repo_dir" clean -fd
fi
[ -f "$repo_dir/scripts/install-system.sh" ] || die "$repo_dir is not a Velora DNS checkout; set VELORA_INSTALL_DIR to an empty directory or a checkout"

go_version=1.27.1
current_go_version=$(go env GOVERSION 2>/dev/null | sed 's/^go//' || true)
if [ "$(printf '%s\n%s\n' "$go_version" "$current_go_version" | sort -V | head -n 1)" != "$go_version" ]; then
  tmp_dir=$(mktemp -d)
  trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
  go_archive="go${go_version}.linux-${platform_arch}.tar.gz"
  printf '%s\n' "Installing Go $go_version…"
  curl -fsSLo "$tmp_dir/$go_archive" "https://go.dev/dl/$go_archive"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "$tmp_dir/$go_archive"
fi
export PATH="/usr/local/go/bin:$PATH"

node_major=24
current_node_major=$(node --version 2>/dev/null | sed 's/^v//' | cut -d. -f1 || true)
if [ "$current_node_major" != "$node_major" ]; then
  tmp_dir=${tmp_dir:-$(mktemp -d)}
  trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
  node_base_url="https://nodejs.org/dist/latest-v${node_major}.x"
  node_archive=$(curl -fsSL "$node_base_url/SHASUMS256.txt" | awk "/node-v${node_major}\\.[0-9.]*-linux-${node_arch}\\.tar\\.xz$/ { print \$2; exit }")
  [ -n "$node_archive" ] || die "could not determine the latest Node.js ${node_major} archive"
  node_checksum=$(curl -fsSL "$node_base_url/SHASUMS256.txt" | awk -v archive="$node_archive" '$2 == archive { print $1; exit }')
  [ -n "$node_checksum" ] || die "could not determine the Node.js archive checksum"
  printf '%s\n' "Installing Node.js $node_major…"
  curl -fsSLo "$tmp_dir/$node_archive" "$node_base_url/$node_archive"
  printf '%s  %s\n' "$node_checksum" "$tmp_dir/$node_archive" | sha256sum -c -
  node_dir="/opt/${node_archive%.tar.xz}"
  sudo rm -rf "$node_dir"
  sudo tar -C /opt -xJf "$tmp_dir/$node_archive"
  for binary in node npm npx corepack; do
    sudo ln -sfn "$node_dir/bin/$binary" "/usr/local/bin/$binary"
  done
fi

exec "$repo_dir/scripts/install-system.sh"
