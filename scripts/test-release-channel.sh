#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
. "$script_dir/release-channel.sh"
failures=0
assert_equal(){ [ "$1" = "$2" ] || { echo "expected '$2', got '$1'"; failures=$((failures+1)); }; }
assert_pass(){ if ! "$@" >/dev/null 2>&1; then echo "expected pass: $*"; failures=$((failures+1)); fi; }
assert_fail(){ if "$@" >/dev/null 2>&1; then echo "expected fail: $*"; failures=$((failures+1)); fi; }

# Channel detection tests.
assert_equal "$(channel_for_version 1.0.0)" "stable"
assert_equal "$(channel_for_version 1.0.0-beta.1)" "beta"
assert_equal "$(channel_for_version 1.0.0-beta.2)" "beta"
assert_equal "$(channel_for_version 1.0.0-alpha.1)" "alpha"
assert_equal "$(channel_for_version 1.0.0-alpha.beta.1)" "alpha"
assert_equal "$(channel_for_version 1.0.0-rc.1)" "stable"

# Eligibility: stable channel blocks prereleases.
assert_fail check_release_eligibility 1.1.0-beta.1 1.0.0 1 false stable
assert_pass check_release_eligibility 1.1.0 1.0.0 1 false stable

# Eligibility: prerelease allowed with explicit opt-in.
assert_pass check_release_eligibility 1.1.0-beta.1 1.0.0 1 true beta

# Eligibility: downgrade blocked.
assert_fail check_release_eligibility 0.9.0 1.0.0 1 false stable
assert_fail check_release_eligibility 1.0.0 1.0.0 1 false stable

# Eligibility: same version blocked (not strictly newer).
assert_fail check_release_eligibility 1.0.0 1.0.0 1 false stable

# Eligibility: channel mismatch.
assert_fail check_release_eligibility 1.1.0-beta.1 1.0.0 1 true stable
assert_fail check_release_eligibility 1.1.0 1.0.0 1 false beta

# Eligibility: no current version (fresh install).
assert_pass check_release_eligibility 1.0.0 "" 1 false stable

# Release metadata parsing from version (no JSON file).
metadata=$(parse_release_metadata_from_version 1.0.0-beta.2)
assert_equal "$(printf '%s\n' "$metadata" | sed -n 's/^version=//p')" "1.0.0-beta.2"
assert_equal "$(printf '%s\n' "$metadata" | sed -n 's/^channel=//p')" "beta"

# Release metadata JSON validation.
temp_dir=$(mktemp -d)
trap 'rm -rf "$temp_dir"' EXIT
cat > "$temp_dir/good.json" <<'EOF'
{
  "version": "1.0.0",
  "channel": "stable",
  "schema_version": 1,
  "min_schema_version": 1,
  "architectures": ["linux/amd64", "linux/arm64"]
}
EOF
assert_pass validate_release_metadata "$temp_dir/good.json"

cat > "$temp_dir/bad_channel.json" <<'EOF'
{
  "version": "1.0.0-beta.1",
  "channel": "stable",
  "schema_version": 1,
  "min_schema_version": 1,
  "architectures": ["linux/amd64"]
}
EOF
assert_fail validate_release_metadata "$temp_dir/bad_channel.json"

cat > "$temp_dir/bad_arch.json" <<'EOF'
{
  "version": "1.0.0",
  "channel": "stable",
  "schema_version": 1,
  "min_schema_version": 1,
  "architectures": ["linux/armv7"]
}
EOF
assert_fail validate_release_metadata "$temp_dir/bad_arch.json"

cat > "$temp_dir/bad_schema.json" <<'EOF'
{
  "version": "1.0.0",
  "channel": "stable",
  "schema_version": 1,
  "min_schema_version": 2,
  "architectures": ["linux/amd64"]
}
EOF
assert_fail validate_release_metadata "$temp_dir/bad_schema.json"

cat > "$temp_dir/bad_version.json" <<'EOF'
{
  "version": "not-a-version",
  "channel": "stable",
  "schema_version": 1,
  "min_schema_version": 1,
  "architectures": ["linux/amd64"]
}
EOF
assert_fail validate_release_metadata "$temp_dir/bad_version.json"

cat > "$temp_dir/minimal.json" <<'EOF'
{
  "version": "2.0.0-alpha.1"
}
EOF
assert_pass validate_release_metadata "$temp_dir/minimal.json"

[ "$failures" -eq 0 ] || exit 1
echo "release channel policy tests passed"
