#!/bin/sh
# Build a native release bundle for a specific architecture.
# Usage: build-release-bundle.sh VERSION ARCH OUTPUT_DIR
# Example: build-release-bundle.sh 1.0.0 amd64 dist/
set -eu

die() { printf '%s\n' "error: $*" >&2; exit 1; }

usage() {
  echo "usage: build-release-bundle.sh VERSION ARCH OUTPUT_DIR" >&2
  echo "  ARCH must be amd64 or arm64" >&2
  exit 2
}

[ "$#" -eq 3 ] || usage
version=$1
arch=$2
output_dir=$3

case "$arch" in
  amd64|arm64) ;;
  *) die "unsupported architecture: $arch (must be amd64 or arm64)" ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
cd "$repo_dir"

[ -n "$version" ] || die "version is required"
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$' || die "invalid version format: $version"

printf '%s\n' "Building release bundle for linux/$arch..."

# Build the frontend.
npm --prefix web ci
npm --prefix web run build

# Build the Go binary for the target architecture.
bundle_name="velora-dns-${version}-linux-${arch}"
bundle_dir="${output_dir}/${bundle_name}"
mkdir -p "$bundle_dir"

CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
  -ldflags "-s -w -X main.version=${version} -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.built=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o "$bundle_dir/velora-dns" ./cmd/server

# Copy web assets.
cp -R web/dist "$bundle_dir/web-dist"

# Copy systemd unit templates.
cat > "$bundle_dir/velora-dns.service" <<'SERVICEEOF'
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
ReadWritePaths=/var/lib/velora /etc/velora

[Install]
WantedBy=multi-user.target
SERVICEEOF

# Copy installation helper.
cat > "$bundle_dir/install.sh" <<'INSTALLEOF'
#!/bin/sh
# Quick installer for the Velora DNS native bundle.
set -eu
die() { printf '%s\n' "error: $*" >&2; exit 1; }
command -v sudo >/dev/null 2>&1 || die "sudo is required"
command -v systemctl >/dev/null 2>&1 || die "systemd is required"

bundle_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
sudo install -d -o root -g root -m 0755 /opt/velora /etc/velora
if ! getent group velora >/dev/null; then sudo groupadd --system velora; fi
if ! id -u velora >/dev/null 2>&1; then
  sudo useradd --system --home-dir /var/lib/velora --shell /usr/sbin/nologin --gid velora velora
fi
sudo install -d -o velora -g velora -m 0750 /var/lib/velora
sudo install -o root -g root -m 0755 "$bundle_dir/velora-dns" /opt/velora/velora-dns
sudo rm -rf /opt/velora/web
sudo install -d -o root -g root -m 0755 /opt/velora/web
sudo cp -R "$bundle_dir/web-dist" /opt/velora/web/dist
sudo chown -R root:root /opt/velora/web
sudo install -o root -g root -m 0644 "$bundle_dir/velora-dns.service" /etc/systemd/system/velora-dns.service
sudo systemctl daemon-reload
printf '%s\n' "Installed Velora DNS ${version} for linux/${arch}"
printf '%s\n' "Create /etc/velora/config.yaml and /etc/velora/velora.env, then: sudo systemctl enable --now velora-dns"
INSTALLEOF
chmod +x "$bundle_dir/install.sh"

# Copy release metadata if present.
if [ -f "$repo_dir/release-metadata.json" ]; then
  cp "$repo_dir/release-metadata.json" "$bundle_dir/"
fi

# Create checksums.
cd "$output_dir"
sha256sum "$bundle_name/velora-dns" > "${bundle_name}.sha256sum"
cat "${bundle_name}.sha256sum"

printf '%s\n' "Bundle created: $bundle_dir"
