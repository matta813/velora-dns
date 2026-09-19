# Release workflow

**Publication is disabled and no release has been made.** Repository variable
`RELEASE_ENABLED=false` gates the Release workflow's first job. It must remain disabled
until the maintainer explicitly authorizes publishing. Do not create tags, releases or
registry images as part of normal development. `RELEASE` is preparation metadata only.

The pipeline follows the administrative reference: a future reviewed PR changes `RELEASE`,
containing one SemVer line without `v`. After explicit enablement, only a main push changing
that file triggers publication; tag pushes do not. Feature merges leave RELEASE unchanged.

Validation resolves a single source commit and its timestamp, runs backend/frontend and
release-infrastructure tests, then anchors an immutable `vVERSION` tag. The build publishes
versioned GHCR images with source/revision/version/timestamp labels and provenance. The
GitHub Release targets the same commit. Only a successful stable release promotes that
exact image digest to `latest`; prereleases never promote it.

Recovery dispatch accepts an existing tag, resolves its original commit, verifies RELEASE
and all existing image/release metadata, and repairs missing stages. It never moves a tag
or rebuilds unverifiable existing immutable artifacts. The source is not current main.

Release notes derive from merged PRs, group Conventional Commit titles, preserve contributor
links and exclude the release PR and `skip-changelog` entries. Scripts support deterministic
preview/regeneration; editing a published body requires explicit `--apply`. Announcement and
weekly report workflows only prepare downloadable drafts and never post externally.

Only job-scoped short-lived `GITHUB_TOKEN` permissions are needed; no long-lived secret,
release environment or extra collaborator was configured in the reference. Workflows use
immutable Action pins. Local builds report `dev`, `unknown`, `unknown`; build args are
`VERSION`, `COMMIT_SHA`, `BUILD_TIME`, visible through `/api/v1/version`.

Run `make release-test` to validate logic without creating any publication. These tests
create disposable local Git repositories and fixture tags only; they never tag this project
or push test tags. The disabled publication gate is also tested.

## Release channel policy

Releases follow semantic versioning and are classified into three channels based on their
pre-release suffix:

| Channel    | Example            | Pre-release tag | Automatic selection |
|------------|--------------------|-----------------|---------------------|
| **stable** | `1.0.0`            | none            | Yes                 |
| **beta**   | `1.0.0-beta.1`     | `-beta.N`       | Opt-in only         |
| **alpha**  | `1.0.0-alpha.1`    | `-alpha.N`      | Opt-in only         |

Release candidates (`-rc.N`) are classified as **stable** because they represent the final
validation step before a stable release.

### Eligibility rules

- **Stable installations never select prereleases automatically.** An administrator must
  explicitly opt in to receive beta or alpha updates.
- **Downgrades are blocked.** A release must have a strictly higher SemVer than the
  currently installed version.
- **Schema compatibility is enforced.** Each release declares a `min_schema_version` that
  must be less than or equal to the installed schema version. This prevents applying
  releases that require database migrations not yet present on the host.

### Release metadata

An optional `release-metadata.json` file provides structured metadata alongside the
`RELEASE` file:

```json
{
  "version": "1.0.0",
  "channel": "stable",
  "schema_version": 1,
  "min_schema_version": 1,
  "architectures": ["linux/amd64", "linux/arm64"]
}
```

| Field                | Description                                               |
|----------------------|-----------------------------------------------------------|
| `version`            | SemVer version (must match `RELEASE`)                     |
| `channel`            | `stable`, `beta`, or `alpha` (auto-detected if omitted)   |
| `schema_version`     | Database schema version of this release                   |
| `min_schema_version` | Minimum schema version required to install this release   |
| `architectures`      | Supported platforms (currently `linux/amd64`, `linux/arm64`) |

See `release-metadata.json.example` for a reference file. The `validate-release-channel.sh`
script validates metadata consistency, and `test-release-channel.sh` covers the eligibility
rules and channel detection logic.

## Release assets

Each release publishes native installation bundles for Linux amd64 and arm64. The bundles
contain:

- **velora-dns** – Statically linked binary (CGO_ENABLED=0)
- **web-dist/** – Compiled React dashboard
- **velora-dns.service** – Hardened systemd unit template
- **install.sh** – Quick installer script
- **release-metadata.json** – Channel and architecture metadata (if present)

### Bundle naming

```
velora-dns-VERSION-linux-ARCH.tar.gz
```

Example: `velora-dns-1.0.0-linux-amd64.tar.gz`

### Checksum verification

Every bundle has a corresponding `.sha256sum` file. A unified `CHECKSUMS.sha256` manifest
is published alongside the bundles. Verify before installing:

```bash
sha256sum -c CHECKSUMS.sha256 --ignore-missing
```

The release workflow verifies all checksums before publishing. Each bundle is uploaded as a
GitHub Release asset with provenance attestation.

### Building bundles locally

```bash
make release-bundle VERSION=1.0.0 ARCH=amd64
```

Or build both architectures:

```bash
./scripts/build-release-bundle.sh 1.0.0 amd64 dist-bundles
./scripts/build-release-bundle.sh 1.0.0 arm64 dist-bundles
./scripts/verify-release-checksums.sh dist-bundles
./scripts/generate-checksums-manifest.sh dist-bundles dist-bundles/CHECKSUMS.sha256
```

## Transactional updates

All update mechanisms (systemd agent, Compose agent) follow a shared transactional contract
defined in `internal/update/update.go`. This ensures consistent behavior regardless of the
deployment mode.

### Update states

```
idle → downloading → verifying → installing → readiness → completed
                      ↓            ↓            ↓
                     failed       failed      rolled_back
                                              (→ completed after rollback)
```

### State transitions

| From          | Allowed transitions                |
|---------------|-------------------------------------|
| `idle`        | `downloading`                       |
| `downloading` | `verifying`, `failed`               |
| `verifying`   | `installing`, `failed`              |
| `installiness`| `readiness`, `failed`               |
| `readiness`   | `completed`, `rolled_back`, `failed`|

### Readiness gates

After installation, the updater waits for the service to become ready before declaring
success. The readiness check interval and timeout are configurable:

- **ReadinessTimeout**: Maximum wait time (default: 5 minutes)
- **ReadinessInterval**: Check interval (default: 5 seconds)

If readiness fails or times out, the updater automatically rolls back to the previous
version.

### Durable state

Update state is persisted to disk (default: `/var/lib/velora/update-state.json`) so that
in-progress updates can be recovered after a process restart. The state file includes:

- Current update entry (if in progress)
- Update history (bounded to `MaxHistory` entries)

### Update history

Every update attempt is recorded in the history with:

- Timestamps (started, completed)
- Version transition (from → to)
- Final state and any error messages
- Whether rollback was used
- Deployment mode (systemd, compose)

### CLI tool

The `updatectl` command provides a CLI for interacting with the update manager:

```bash
# Check current status
updatectl status

# Begin an update
updatectl begin 0.1.0 0.2.0 systemd

# Transition state
updatectl transition <ID> verifying
updatectl transition <ID> installing

# Mark completion or failure
updatectl complete <ID>
updatectl fail <ID> "checksum mismatch"
updatectl rollback <ID>

# View history
updatectl history
```

### Integration tests

The update manager includes comprehensive tests covering:

- Successful update flow (all states)
- Concurrent update rejection
- Invalid state transitions
- Failure and rollback
- History pruning with MaxHistory
- State persistence and recovery
- Readiness configuration

## Systemd updater agent

The `velora-updater` binary is a root-owned systemd update agent for native installations.
It runs as a separate privileged service and accepts update requests from the unprivileged
Velora DNS process over a Unix socket.

### Security model

- The agent runs as **root** with `CAP_NET_BIND_SERVICE` only
- Socket ownership: root:velora with permissions `0660`
- The Velora DNS service (running as user `velora`) is the only client
- No arbitrary commands, URLs, paths, or environment variables from HTTP clients
- Only release assets from the configured GitHub repository are downloaded
- SHA-256 checksums are verified before installation

### Installation

The updater agent is included in native release bundles and installed by the installer script:

```bash
# Install the updater agent
sudo install -o root -g root -m 0755 velora-updater /opt/velora/velora-updater
sudo install -o root -g root -m 0644 velora-updater.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now velora-updater
```

### Configuration

The agent is configured via environment variables:

| Variable                  | Default                              | Description                        |
|---------------------------|--------------------------------------|------------------------------------|
| `VELORA_UPDATER_SOCKET`   | `/run/velora-updater.sock`           | Unix socket path                   |
| `VELORA_UPDATE_STATE_FILE`| `/var/lib/velora/update-state.json`  | Durable state file                 |
| `VELORA_BINARY_PATH`      | `/opt/velora/velora-dns`             | Velora DNS binary path              |
| `VELORA_WEB_DIR`          | `/opt/velora/web`                    | Web assets directory                |
| `VELORA_REPOSITORY`       | `matta813/velora-dns`                | GitHub repository for releases      |
| `VELORA_READINESS_URL`    | `http://127.0.0.1:8080/ready`        | Readiness check endpoint            |
| `VELORA_CHANNEL`          | `stable`                             | Release channel (`stable`, `beta`, `alpha`) |

### Systemd unit hardening

```ini
[Service]
ExecStart=/opt/velora/velora-updater
NoNewPrivileges=false
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/velora /run
SupplementaryGroups=velora
```

### Update flow

1. Velora DNS sends `POST /update` with `{"action":"update"}` to the Unix socket
2. Agent validates the request and begins a transaction
3. Agent resolves the latest release from GitHub Releases
4. Agent verifies the SHA-256 checksum of the downloaded bundle
5. Agent backs up current binary and web assets (`.prev` suffix)
6. Agent installs the new binary and web assets
7. Agent waits for readiness (up to 5 minutes)
8. On success: agent restarts `velora-dns` service
9. On failure: agent restores from backup and rolls back

### Status endpoint

```bash
# Check agent status
curl --unix-socket /run/velora-updater.sock http://localhost/status

# Trigger an update
curl -X POST --unix-socket /run/velora-updater.sock \
  -H "Content-Type: application/json" \
  -d '{"action":"update"}' \
  http://localhost/update
```
