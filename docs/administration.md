# Repository administration

The administrative baseline follows PGSentinel, adapted to Velora DNS.

- Main: PRs required, linear history, resolved conversations, no force pushes or deletion;
  protection includes administrators. No mandatory approval count, matching the reference.
- Squash merges only; PR title/body become commit title/body; merged branches deleted.
- Issues and Discussions enabled; Wiki and Projects disabled; CODEOWNERS: @matta813.
- Actions default to read-only and cannot approve PRs. Publication permissions are job-scoped.
- Private vulnerability reporting, Dependabot alerts/updates, secret scanning and push protection.
- Daily Dependabot at 04:00 Europe/Zurich for Go, npm, Docker and Actions; patch/minor/major
  groups retained, five open PRs per ecosystem, no automatic merging.
- SHA-pinned Actions, CodeQL, dependency review, Go vulnerability scans and OpenSSF Scorecard.
- Release metadata, immutable tags/images, deterministic notes and recovery scripts retained.

**Release publication is disabled.** `RELEASE_ENABLED` is unset/false; the release validation
job cannot run. No releases or image publications are part of initial implementation.
Enabling publication requires a later explicit maintainer decision. Normal feature PRs
must not change RELEASE. Announcement and growth jobs create reviewable artifacts only.

Required checks mirror the reference: release-logic, backend, frontend, build,
Review dependency changes, Scan Go dependencies, Analyze (go), Analyze (javascript-typescript).
No extra rulesets, environments, collaborators or release secrets existed in the reference.
