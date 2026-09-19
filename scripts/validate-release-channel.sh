#!/bin/sh
# Validate release metadata and channel policy for a given RELEASE file or release-metadata.json.
set -eu
. "$(dirname "$0")/release-channel.sh"

usage() {
  echo "usage: validate-release-channel.sh [--json METADATA_FILE] [RELEASE_FILE]" >&2
  exit 2
}

json_file=""
release_file=""

while [ $# -gt 0 ]; do
  case "$1" in
    --json)
      shift; json_file=${1:-}
      [ -n "$json_file" ] || { echo "--json requires a file path" >&2; exit 2; }
      ;;
    -*) echo "unknown option: $1" >&2; exit 2 ;;
    *)  release_file=$1 ;;
  esac
  shift
done

release_file=${release_file:-RELEASE}
failures=0

# Validate the RELEASE file SemVer.
if [ -f "$release_file" ]; then
  version=$(trim_version "$release_file")
  if ! validate_semver "$version"; then
    echo "invalid SemVer in $release_file: $version" >&2
    failures=$((failures+1))
  else
    printf 'RELEASE version: %s (channel: %s)\n' "$version" "$(channel_for_version "$version")"
  fi
fi

# Validate release-metadata.json if provided.
if [ -n "$json_file" ]; then
  if ! validate_release_metadata "$json_file"; then
    failures=$((failures+1))
  else
    printf 'release metadata valid: %s\n' "$json_file"
  fi
fi

# Cross-validate: if both exist, version in metadata must match RELEASE.
if [ -f "$release_file" ] && [ -n "$json_file" ] && [ -f "$json_file" ]; then
  release_version=$(trim_version "$release_file")
  metadata=$(parse_release_metadata "$json_file") || { failures=$((failures+1)); }
  if [ -n "$metadata" ]; then
    meta_version=$(printf '%s\n' "$metadata" | sed -n 's/^version=//p')
    if [ "$release_version" != "$meta_version" ]; then
      echo "version mismatch: RELEASE=$release_version metadata=$meta_version" >&2
      failures=$((failures+1))
    fi
  fi
fi

[ "$failures" -eq 0 ] || exit 1
printf 'release channel validation passed\n'
