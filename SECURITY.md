# Security policy

This project is pre-release and has not undergone a security audit. Only the latest
main branch receives fixes. The management API uses password-authenticated revocable
sessions, CSRF protection and server-enforced roles. Keep it on loopback or a trusted
management network as defense in depth. Default listeners bind to loopback and DNS
clients are restricted by CIDR. Docker publishes ports on host loopback only.

Use GitHub private vulnerability reporting on this repository to disclose issues.
If private reporting is unavailable, open an issue requesting a private contact
without including exploit details or secrets. Never disclose vulnerabilities in
public query logs. Public recursive DNS operation is outside the supported scope.
