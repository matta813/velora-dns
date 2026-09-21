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
temp_file="${output_file}.tmp"
> "$temp_file"
for archive in *.tar.gz; do
  [ -f "$archive" ] || continue
  sha256sum "$archive" >> "$temp_file"
done
[ -s "$temp_file" ] || die "no release archives found"
mv "$temp_file" "$output_file"

printf '%s\n' "Checksums manifest: $output_file"
cat "$output_file"
