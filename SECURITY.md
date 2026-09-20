# Security policy

Velora DNS is in beta and has not had an independent security audit. Treat it as
security-sensitive infrastructure: keep the management interface behind trusted
network access and limit forwarding to known client CIDRs. See the
[deployment guide](docs/deployment.md) for exposure and backup guidance.

## Supported versions

Security fixes target the current `main` branch and, when practical, the latest
published beta pre-release. Older pre-releases and development snapshots are not
maintained. Check [Releases](https://github.com/matta813/velora-dns/releases) for the
latest published version; there is no stable release support promise yet.

## Report a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/matta813/velora-dns/security/advisories/new).
Include the affected version or commit, configuration and exposure conditions,
reproduction steps, expected and actual behavior, and impact. Redact credentials,
query data, and other private information. Please give the maintainer a chance to
investigate before disclosing details publicly.

If private reporting is unavailable, open a public issue requesting a private
contact method **without including exploit details**. The maintainer will acknowledge
and triage reports as capacity permits, coordinate a fix and disclosure where
appropriate, and credit reporters who want attribution. No fixed response or repair
time is promised. For ordinary bugs and feature requests, use
[GitHub Issues](https://github.com/matta813/velora-dns/issues).
