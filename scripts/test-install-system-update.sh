#!/usr/bin/env sh
# Verify the systemd installer's update path: an existing binary is backed up
# and the installer reports an update instead of a fresh install.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
mock_bin="$tmp_dir/bin"
log_file="$tmp_dir/commands.log"
fake_root="$tmp_dir/fakeroot"
mkdir -p "$mock_bin" "$tmp_dir/remote" "$fake_root/opt/velora" "$fake_root/etc/velora"

fail() { printf '%s\n' "test failure: $*" >&2; cat "$log_file" >&2; exit 1; }

# translate maps absolute installer paths into the fake root.
translate() {
  case "$1" in
    /opt/velora*) printf '%s' "$fake_root$1" ;;
    /etc/velora*) printf '%s' "$fake_root$1" ;;
    *) printf '%s' "$1" ;;
  esac
}

cat > "$mock_bin/sudo" <<'EOF'
#!/bin/sh
set -eu
printf 'sudo %s\n' "$*" >> "$TEST_INSTALLER_LOG"
fake_root=${TEST_INSTALLER_FAKE_ROOT:?}
case "$1" in
  test)
    shift
    if [ "$1" = -x ]; then
      [ -x "$fake_root$2" ]
      exit $?
    fi
    [ -f "$fake_root$2" ]
    ;;
  cp)
    shift
    # Skip flags like -R/-r/-a.
    while [ "$#" -gt 0 ] && [ "${1#-}" != "$1" ]; do shift; done
    src=$1
    dst=$2
    case "$src" in
      /opt/velora*|/etc/velora*) src="$fake_root$src" ;;
    esac
    cp -R "$src" "$fake_root$dst"
    ;;
  install)
    shift
    # install [OPTIONS]... SOURCE DEST  or  install -d [OPTIONS]... DIR
    is_dir=false
    mode=0755
    positional=""
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -d) is_dir=true ;;
        -o|-g) shift ;;
        -m) shift; mode=$1 ;;
        -*) ;;
        *) positional="$positional $1" ;;
      esac
      shift
    done
    # positional is "SOURCE DEST" or "DIR" (dir mode).
    set -- $positional
    if [ "$is_dir" = true ]; then
      mkdir -p "$fake_root$1"
    else
      target=$1
      dest=$2
      mkdir -p "$(dirname "$fake_root$dest")"
      cp "$target" "$fake_root$dest"
      chmod "$mode" "$fake_root$dest"
    fi
    ;;
  rm)
    shift
    for arg in "$@"; do
      case "$arg" in
        -rf|-r|-f) ;;
        *) rm -rf "$fake_root$arg" ;;
      esac
    done
    ;;
  chown|chmod)
    shift
    ;;
  tee)
    shift
    # Read stdin into the translated file.
    dest=""
    for arg in "$@"; do dest="$arg"; done
    mkdir -p "$(dirname "$fake_root$dest")"
    cat > "$fake_root$dest"
    ;;
  groupadd|useradd)
    shift
    ;;
  systemctl)
    # Delegated to the mock systemctl in $mock_bin.
    exec systemctl "$@"
    ;;
  *) ;;
esac
exit 0
EOF

cat > "$mock_bin/go" <<'EOF'
#!/bin/sh
if [ "$1" = env ]; then shift; fi
if [ "$1" = GOVERSION ]; then printf '%s\n' go1.27.1; exit 0; fi
if [ "$1" = build ]; then
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "-o" ]; then shift; printf 'new-binary' > "$1"; chmod +x "$1"; fi
    shift
  done
  exit 0
fi
exit 0
EOF

cat > "$mock_bin/npm" <<'EOF'
#!/bin/sh
printf 'npm %s\n' "$*" >> "$TEST_INSTALLER_LOG"
# Pretend the dashboard was built so the installer can copy it.
mkdir -p web/dist
printf '<html>fake dashboard</html>' > web/dist/index.html
exit 0
EOF

cat > "$mock_bin/openssl" <<'EOF'
#!/bin/sh
printf '%s\n' generated-password-which-is-long-enough
EOF

cat > "$mock_bin/systemctl" <<'EOF'
#!/bin/sh
fake_root=${TEST_INSTALLER_FAKE_ROOT:?}
log=${TEST_INSTALLER_LOG:?}
shift
case "$1" in
  is-active)
    # Report running when the binary exists (update scenario).
    [ -x "$fake_root/opt/velora/velora-dns" ] && exit 0
    exit 1
    ;;
  stop)
    printf 'systemctl stop %s\n' "$2" >> "$log"
    ;;
  daemon-reload|enable|restart|status) ;;
esac
exit 0
EOF

for t in getent id ss; do
  cat > "$mock_bin/$t" <<'EOF'
#!/bin/sh
exit 1
EOF
done

chmod +x "$mock_bin"/*
cp "$root/scripts/install-system.sh" "$tmp_dir/remote/install-system.sh"
mkdir -p "$tmp_dir/scripts"
cp "$root/scripts/velora-updater.service" "$tmp_dir/scripts/velora-updater.service"

# A fake already-installed binary; the installer must back it up.
printf 'old-binary' > "$fake_root/opt/velora/velora-dns"
chmod +x "$fake_root/opt/velora/velora-dns"

# Existing bootstrap env makes the installer keep credentials and skip prompts.
mkdir -p "$fake_root/etc/velora"
cat > "$fake_root/etc/velora/velora.env" <<'EOF'
VELORA_BOOTSTRAP_USERNAME=admin
VELORA_BOOTSTRAP_PASSWORD=existing-password-that-is-long
EOF

# Existing config so the installer updates it in place.
cat > "$fake_root/etc/velora/config.yaml" <<'EOF'
dns:
  listen: ['127.0.0.1:53']
http:
  listen: '127.0.0.1:8080'
  allowed_hosts: ['localhost', '127.0.0.1', '::1']
EOF

TEST_INSTALLER_LOG="$log_file" TEST_INSTALLER_FAKE_ROOT="$fake_root" \
PATH="$mock_bin:$PATH" VELORA_INSTALL_DIR="$tmp_dir/source" \
  VELORA_CHANNEL=stable VELORA_HTTP_HOST=127.0.0.1 VELORA_DNS_HOST=127.0.0.1 \
  sh "$tmp_dir/remote/install-system.sh" > "$tmp_dir/output.log" 2>&1 || {
  printf '%s\n' "installer failed:" >&2
  cat "$tmp_dir/output.log" >&2
  cat "$log_file" >&2
  exit 1
}

# The old binary must have been backed up before the new one replaced it.
[ -f "$fake_root/opt/velora/velora-dns.old" ] || fail "expected backup of existing binary"
[ "$(cat "$fake_root/opt/velora/velora-dns")" = "new-binary" ] || fail "binary was not replaced"
grep -q "Existing Velora DNS installation detected; updating it" "$tmp_dir/output.log" || fail "expected update message"
grep -q "Stopping running Velora DNS service" "$tmp_dir/output.log" || fail "expected service stop message"
grep -q "systemctl stop velora-dns" "$log_file" || fail "systemctl stop was not called during update"
grep -q "Existing installation updated" "$tmp_dir/output.log" || fail "expected update summary"

printf '%s\n' 'Systemd installer update test passed'
