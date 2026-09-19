#!/bin/sh
# Verify checksums for all release bundles in a directory.
# Usage: verify-release-checksums.sh BUNDLE_DIR
set -eu

die() { printf '%s\n' "error: $*" >&2; exit 1; }

[ "$#" -eq 1 ] || { echo "usage: verify-release-checksums.sh BUNDLE_DIR" >&2; exit 2; }
bundle_dir=$1

[ -d "$bundle_dir" ] || die "bundle directory not found: $bundle_dir"

cd "$bundle_dir"
failures=0
for checksum_file in *.sha256sum; do
  [ -f "$checksum_file" ] || continue
  if sha256sum -c "$checksum_file" >/dev/null 2>&1; then
    printf 'OK: %s\n' "$checksum_file"
  else
    printf 'FAILED: %s\n' "$checksum_file" >&2
    failures=$((failures+1))
  fi
done

[ "$failures" -eq 0 ] || { printf '%s\n' "$failures checksum(s) failed" >&2; exit 1; }
printf '%s\n' "All checksums verified"
