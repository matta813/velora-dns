#!/usr/bin/env sh
# Verify the Compose installer workflow without touching the host.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
mock_bin="$tmp_dir/bin"
log_file="$tmp_dir/commands.log"
mkdir -p "$mock_bin" "$tmp_dir/remote"

fail() { printf '%s\n' "test failure: $*" >&2; cat "$log_file" >&2; exit 1; }
assert_logged() { grep -Fqx -- "$1" "$log_file" || fail "expected command: $1"; }

cat > "$mock_bin/sudo" <<'EOF'
#!/bin/sh
set -eu
printf 'sudo %s\n' "$*" >> "$TEST_INSTALLER_LOG"
if [ "$1" = git ] && [ "$2" = clone ]; then
  for target; do :; done
  mkdir -p "$target"
  : > "$target/docker-compose.yml"
  exit 0
fi
if [ "$1" = env ]; then shift; exec env "$@"; fi
if [ "$1" = install ]; then
  exit 0
fi
if [ "$1" = mktemp ]; then
  case "$2" in
    /run/velora/*) echo "$MOCK_COMPOSE_ENV" ;;
    *) echo "$MOCK_UPDATER_TMP" ;;
  esac
  exit 0
fi
if [ "$1" = chmod ]; then exit 0; fi
if [ "$1" = rm ]; then exit 0; fi
if [ "$1" = tee ]; then cat >/dev/null; exit 0; fi
exit 0
EOF

cat > "$mock_bin/docker" <<'EOF'
#!/bin/sh
printf 'docker %s\n' "$*" >> "$TEST_INSTALLER_LOG"
[ "$1" = compose ]
EOF

cat > "$mock_bin/openssl" <<'EOF'
#!/bin/sh
printf '%s\n' generated-password-which-is-long-enough
EOF

cat > "$mock_bin/getent" <<'EOF'
#!/bin/sh
printf '%s\n' 'velora:x:10001:'
EOF

chmod +x "$mock_bin"/*
cp "$root/scripts/install-compose.sh" "$tmp_dir/remote/install-compose.sh"

TEST_INSTALLER_LOG="$log_file" MOCK_COMPOSE_ENV="$tmp_dir/compose.env" MOCK_UPDATER_TMP="$tmp_dir/updater" PATH="$mock_bin:$PATH" VELORA_INSTALL_DIR="$tmp_dir/source" VELORA_CHANNEL=beta VELORA_HTTP_HOST=0.0.0.0 VELORA_DNS_HOST=0.0.0.0 \
  sh "$tmp_dir/remote/install-compose.sh"

assert_logged "sudo docker compose --env-file $tmp_dir/compose.env build"
assert_logged "sudo docker create velora-dns:local"
assert_logged "sudo systemctl enable --now velora-compose-updater"
assert_logged "sudo docker compose --env-file $tmp_dir/compose.env up -d"
assert_logged "sudo docker compose ps"
printf '%s\n' 'Compose installer test passed'
