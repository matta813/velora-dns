# Security policy

This project is pre-release and has not undergone a security audit. Only the latest
main branch receives fixes. Management access uses password-authenticated, revocable
sessions with admin/operator/viewer roles, but should still not be exposed directly to
untrusted networks. Default listeners bind to loopback and DNS
clients are restricted by CIDR. Docker publishes ports on host loopback only.

Use GitHub private vulnerability reporting on this repository to disclose issues.
If private reporting is unavailable, open an issue requesting a private contact
without including exploit details or secrets. Never disclose vulnerabilities in
public query logs. Public recursive DNS operation is outside the supported scope.
