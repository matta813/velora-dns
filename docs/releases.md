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
