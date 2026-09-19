#!/bin/sh
# Generate a checksums manifest for all release bundles in a directory.
# Usage: generate-checksums-manifest.sh BUNDLE_DIR OUTPUT_FILE
set -eu

die() { printf '%s\n' "error: $*" >&2; exit 1; }

[ "$#" -eq 2 ] || { echo "usage: generate-checksums-manifest.sh BUNDLE_DIR OUTPUT_FILE" >&2; exit 2; }
bundle_dir=$1
output_file=$2

[ -d "$bundle_dir" ] || die "bundle directory not found: $bundle_dir"

cd "$bundle_dir"
> "$output_file"
for checksum_file in *.sha256sum; do
  [ -f "$checksum_file" ] || continue
  cat "$checksum_file" >> "$output_file"
done

printf '%s\n' "Checksums manifest: $output_file"
cat "$output_file"
