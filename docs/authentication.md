# Management authentication

New databases require a bootstrap administrator. Set a strong password only for the first
startup; Velora stores an Argon2id hash and never stores or logs the environment value.

```bash
VELORA_BOOTSTRAP_PASSWORD='use-a-unique-password-manager-value' docker compose up --build -d
```

After the administrator exists, remove `VELORA_BOOTSTRAP_PASSWORD` from the environment.
`VELORA_BOOTSTRAP_USERNAME` defaults to `admin`. Passwords must contain 12–1024 bytes;
usernames are 3–64 ASCII letters, digits, dots, underscores or hyphens. At most 256 users
are retained.

Passwords use Argon2id with 64 MiB memory, three iterations, two lanes, and independent
16-byte salts. Login hashing is limited to two concurrent operations; each source address
gets five attempts per five-minute window and the limiter remembers at most 1024 sources.
Authentication errors do not reveal whether a username exists.

Sessions use 32 random bytes and only SHA-256 token hashes are stored. They expire after
`auth.session_ttl` (12 hours by default), are revocable on logout, and are revoked when a
password changes or an account is disabled. A user retains at most 32 live sessions.
The session cookie is HttpOnly and SameSite=Strict. A separate SameSite=Strict token is
bound to the session and must match `X-CSRF-Token` on every modifying API request.

Set `auth.secure_cookies: true` (`VELORA_AUTH_SECURE_COOKIES=true`) whenever management is
served through HTTPS. With this flag, browsers will not send either cookie over plain HTTP.
Keep the backend on loopback behind the TLS reverse proxy and preserve Host/Origin headers.

| Role | Access |
|---|---|
| viewer | Read-only operational, zone, filtering and history APIs |
| operator | Viewer access plus DNS/cache/zone/filtering mutations |
| admin | Operator access plus user lifecycle and audit events |

The Users screen is visible only to administrators. `GET /api/v1/audit?limit=100` returns
up to 500 recent events; storage retains the newest 10,000. Login outcomes, logout, user
changes and authenticated management mutation attempts are recorded without passwords,
hashes, session tokens or CSRF tokens. Audit persistence is required before a management
mutation is dispatched; an unavailable audit store fails closed.
