# ADR 0004: Session-based management authentication and roles

Status: accepted.

Velora requires at least one management user. A new database is bootstrapped exactly once
from an environment-only password; subsequent starts use persisted Argon2id hashes. The
application fails before binding listeners when neither a user nor bootstrap credential
exists. Passwords and hashes never enter API payloads, audit events, or logs.

Browser management uses opaque random sessions rather than long-lived credentials. SQLite
stores SHA-256 token hashes and session-specific CSRF hashes. Session cookies are HttpOnly
and SameSite=Strict; deployments terminating HTTPS enable the Secure flag. All unsafe API
methods require the CSRF header in addition to the cookie. Logout, password replacement,
account disabling, expiry and deletion revoke access server-side.

Authorization belongs to the common API middleware. Viewers are read-only, operators can
change DNS operational state, and administrators additionally manage users and inspect
audit events. Frontend visibility improves usability but is not an authorization boundary.
Health and readiness stay public for orchestration; the operational API and metrics require
a session.

Authentication work and retained state are bounded: two concurrent password hashes, five
attempts per source per five minutes, 1024 remembered sources, 256 users, 32 sessions per
user and 10000 audit events. The login-source table uses fixed-space replacement. Authorized
mutation attempts must be durably audited before handler dispatch, so audit failure closes
the mutation path instead of silently losing security evidence.
