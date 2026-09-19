#!/bin/sh
# Release channel policy: defines channels, compatibility rules and selection logic.
. "$(dirname "$0")/release-lib.sh"

# Current schema version of the running installation.
# Bump this when database migrations or config structure break backward compatibility.
CURRENT_SCHEMA_VERSION=1

# Supported release channels in preference order.
CHANNELS="stable beta alpha"

# Determines the channel of a given SemVer version.
#   stable  – no pre-release suffix (e.g. 1.0.0)
#   beta    – pre-release contains "beta" (e.g. 1.0.0-beta.1)
#   alpha   – pre-release contains "alpha" or any other pre-release tag
channel_for_version() {
  version=$1
  if ! is_prerelease "$version"; then
    echo "stable"
    return
  fi
  lower=$(printf '%s\n' "$version" | tr '[:upper:]' '[:lower:]')
  case "$lower" in
    *-beta.*)   echo "beta" ;;
    *-alpha.*)  echo "alpha" ;;
    *-rc.*)     echo "stable" ;;
    *)          echo "alpha" ;;
  esac
}

# Parses release metadata fields from a JSON release-metadata.json file.
# Outputs key=value pairs: version, channel, schema_version, architectures, min_schema_version.
parse_release_metadata() {
  json_file=$1
  [ -f "$json_file" ] || { echo "release metadata file not found: $json_file" >&2; return 1; }

  version=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$json_file" | head -1)
  channel=$(sed -n 's/.*"channel"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$json_file" | head -1)
  schema_version=$(sed -n 's/.*"schema_version"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p' "$json_file" | head -1)
  min_schema_version=$(sed -n 's/.*"min_schema_version"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p' "$json_file" | head -1)
  architectures=$(sed -n 's/.*"architectures"[[:space:]]*:[[:space:]]*\[\([^]]*\)\].*/\1/p' "$json_file" | tr -d ' "')

  [ -n "$version" ] || { echo "missing version in release metadata" >&2; return 1; }
  [ -n "$channel" ] || channel=$(channel_for_version "$version")
  [ -n "$schema_version" ] || schema_version=$CURRENT_SCHEMA_VERSION
  [ -n "$min_schema_version" ] || min_schema_version=1
  [ -n "$architectures" ] || architectures="linux/amd64,linux/arm64"

  echo "version=$version"
  echo "channel=$channel"
  echo "schema_version=$schema_version"
  echo "architectures=$architectures"
  echo "min_schema_version=$min_schema_version"
}

# Validates that a release metadata file is consistent and well-formed.
validate_release_metadata() {
  json_file=$1
  metadata=$(parse_release_metadata "$json_file") || return 1
  eval "$metadata"

  validate_semver "$version" || { echo "invalid version in release metadata: $version" >&2; return 1; }

  declared_channel=$(channel_for_version "$version")
  if [ "$channel" != "$declared_channel" ]; then
    echo "channel mismatch: version $version should be $declared_channel, got $channel" >&2
    return 1
  fi

  case "$channel" in
    stable|beta|alpha) ;;
    *) echo "unknown channel: $channel" >&2; return 1 ;;
  esac

  if [ "$schema_version" -lt 1 ] 2>/dev/null; then
    echo "schema_version must be >= 1" >&2
    return 1
  fi
  if [ "$min_schema_version" -lt 1 ] 2>/dev/null; then
    echo "min_schema_version must be >= 1" >&2
    return 1
  fi
  if [ "$min_schema_version" -gt "$schema_version" ] 2>/dev/null; then
    echo "min_schema_version ($min_schema_version) cannot exceed schema_version ($schema_version)" >&2
    return 1
  fi
  for arch in $(echo "$architectures" | tr ',' ' '); do
    case "$arch" in
      linux/amd64|linux/arm64) ;;
      *) echo "unsupported architecture: $arch" >&2; return 1 ;;
    esac
  done
}

# Determines whether a candidate release is eligible for a given installation.
# Arguments: candidate_version current_version current_schema_version allow_prerelease target_channel
# Returns 0 if eligible, 1 if blocked. Prints a human-readable reason on failure.
check_release_eligibility() {
  candidate=$1
  current=$2
  current_schema=${3:-$CURRENT_SCHEMA_VERSION}
  allow_prerelease=${4:-false}
  target_channel=${5:-stable}

  candidate_channel=$(channel_for_version "$candidate")

  # Stable installations never select prereleases automatically.
  if [ "$target_channel" = "stable" ] && [ "$candidate_channel" != "stable" ]; then
    if [ "$allow_prerelease" != "true" ]; then
      echo "candidate $candidate is a prerelease; stable channel requires explicit opt-in" >&2
      return 1
    fi
  fi

  # Channel must match the target unless the target is explicitly set to accept all.
  if [ "$target_channel" != "all" ] && [ "$candidate_channel" != "$target_channel" ]; then
    echo "candidate $candidate ($candidate_channel) does not match target channel $target_channel" >&2
    return 1
  fi

  # Downgrades are not allowed.
  if [ -n "$current" ] && [ "$(compare_semver "$candidate" "$current")" -le 0 ]; then
    echo "candidate $candidate is not newer than current $current" >&2
    return 1
  fi

  # Schema compatibility: the candidate's min_schema_version must be <= current schema.
  candidate_metadata=$(parse_release_metadata_from_version "$candidate" 2>/dev/null) || true
  if [ -n "$candidate_metadata" ]; then
    candidate_min_schema=$(printf '%s\n' "$candidate_metadata" | sed -n 's/^min_schema_version=//p')
    if [ -n "$candidate_min_schema" ] && [ "$candidate_min_schema" -gt "$current_schema" ] 2>/dev/null; then
      echo "candidate $candidate requires schema_version >= $candidate_min_schema, but current is $current_schema" >&2
      return 1
    fi
  fi

  return 0
}

# Resolves minimal metadata for a version from its pre-release metadata convention.
# This is a fallback when no explicit release-metadata.json exists.
parse_release_metadata_from_version() {
  version=$1
  channel=$(channel_for_version "$version")
  echo "version=$version"
  echo "channel=$channel"
  echo "schema_version=$CURRENT_SCHEMA_VERSION"
  echo "min_schema_version=1"
  echo "architectures=linux/amd64,linux/arm64"
}
