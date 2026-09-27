# Webhook notifications

Webhooks send important operational events to other systems, such as Home
Assistant, ntfy, a chat bridge or an incident tool. Only administrators can
view and change webhooks, in **System → Webhooks** or through
`/api/v1/webhooks`.

## Events

| Type | Severity | When |
| --- | --- | --- |
| `upstream.unavailable` | warning | An upstream resolver stopped answering; failover is active |
| `upstream.recovered` | info | The upstream answers again |
| `blocklist.refresh_failed` | warning | A scheduled or manual blocklist refresh failed; the previous list stays active |
| `blocklist.refresh_recovered` | info | A failing blocklist refreshed successfully again |
| `backup.created` | info | An encrypted backup was created |
| `backup.failed` | warning | A backup could not be created |
| `config.rollback` | warning or critical | A configuration change was rolled back |
| `webhook.test` | info | Sent only by the Test action |

Each webhook can subscribe to specific types (none selected means all of them)
and a minimum severity. Events are the same ones shown in the notification
center; repeated events there are grouped, but each occurrence is delivered.

## Payload

Deliveries are `POST` requests with a JSON body. The format is versioned; new
fields may be added within version 1, and incompatible changes will use a new
`version`.

```json
{
  "version": 1,
  "id": "3f1c9a0e5b7d2c4a8e6f1b3d",
  "type": "upstream.unavailable",
  "severity": "warning",
  "title": "Upstream unavailable",
  "message": "9.9.9.9:53 is not responding; failover is active.",
  "link": "/",
  "occurred_at": "2026-09-27T12:00:00Z",
  "instance": { "name": "dns-1", "version": "0.1.0" }
}
```

Headers:

- `Content-Type: application/json`
- `User-Agent: Velora-DNS-Webhook/<version>`
- `X-Velora-Event`: the event type
- `X-Velora-Delivery`: the payload `id`, useful to drop duplicate retries
- `Authorization: Bearer <token>` when a token is configured

Payloads never contain passwords, tokens, query logs or configuration values.
`link` is a path in the Velora web interface.

## Delivery

- Delivery runs in the background with a queue of 256 events and at most four
  deliveries at a time. DNS processing never waits for a webhook; if the queue
  is full, events are dropped and a warning is logged.
- Each attempt has a 5 second timeout. Network errors, HTTP 408, 429 and 5xx
  responses are retried after 2, 10 and 30 seconds. Other 4xx responses are
  not retried. Redirects are not followed.
- Any 2xx response counts as success. The list shows the last attempt, its
  result and the number of consecutive failures (kept in memory until restart).
- **Test** sends a `webhook.test` event once, without retries, and shows the
  result immediately. It works for paused webhooks too.

## Security

- URLs must use `http` or `https` and cannot contain user names or passwords.
  Prefer `https` for anything that leaves your network.
- Destinations are checked after DNS resolution, on every delivery, and the
  connection goes to the checked address, so DNS rebinding cannot redirect a
  webhook. Link-local addresses (including cloud metadata services),
  multicast and unspecified addresses are always rejected.
- Private, loopback and CGNAT addresses (for example Home Assistant on
  `192.168.1.5:8123`) are rejected unless **Allow private network** is enabled
  for that webhook.
- The bearer token is write-only. The API reports only `has_token`, and the
  token is never logged. When editing, leave the token out (or `null`) to keep
  it, or send an empty string to remove it. Tokens are stored in the database,
  so protect the database file and backups as you would the configuration.
- Some services put a secret in the URL itself; webhook URLs are therefore
  only visible to administrators.
