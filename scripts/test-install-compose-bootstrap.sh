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

chmod +x "$mock_bin"/*
cp "$root/scripts/install-compose.sh" "$tmp_dir/remote/install-compose.sh"

TEST_INSTALLER_LOG="$log_file" PATH="$mock_bin:$PATH" VELORA_INSTALL_DIR="$tmp_dir/source" \
  sh "$tmp_dir/remote/install-compose.sh"

assert_logged "sudo env VELORA_BOOTSTRAP_USERNAME=admin VELORA_BOOTSTRAP_PASSWORD=generated-password-which-is-long-enough docker compose up --build -d"
assert_logged "sudo docker compose ps"
printf '%s\n' 'Compose installer test passed'
