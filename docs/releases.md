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
