# Query history

Query logging is disabled by default. Enable it using YAML `query_log.enabled: true`
or `VELORA_QUERY_LOG_ENABLED=true`. Compose passes through this environment variable.
New queries are retained; enabling history does not reconstruct earlier traffic.

The DNS handler enqueues timestamp, client IP, domain, type, response code, elapsed
time, source, upstream and cache-hit status without waiting for SQL. A single worker
writes batches of up to 128 records. The queue defaults to 1024 records. Overflow and
failed writes increment `dns_query_log_dropped_total`; failed write/retention operations
increment `dns_query_log_errors_total`. Delivery is best effort, not an audit guarantee.

Retention defaults to seven days and 100000 rows, whichever removes a record first.
The bounds apply transactionally on writes and every minute, including while recording
is disabled. SQLite reuses deleted pages; its file does not shrink after every purge.
The application stops DNS/HTTP before draining the queue, with a five-second drain deadline,
then closes SQLite. Abrupt termination can lose queued records.

Configuration:

| YAML | Environment | Default |
|---|---|---|
| query_log.enabled | VELORA_QUERY_LOG_ENABLED | false |
| query_log.queue_size | VELORA_QUERY_LOG_QUEUE_SIZE | 1024 |
| query_log.retention | VELORA_QUERY_LOG_RETENTION | 168h |
| query_log.max_rows | VELORA_QUERY_LOG_MAX_ROWS | 100000 |

Retention must be 1 minute–365 days; row capacity 1–1000000; queue capacity 1–100000.
The management API exposes these settings without exposing the database path.

## Read history

`GET /api/v1/queries` returns a JSON `data` array, including when empty.
Optional filters: `domain` is a literal case-insensitive substring; `client` is an
exact IP; `type` and `source` are exact. `limit` defaults to 100 and is bounded at 500.
Results are ordered by descending insertion ID. Supply the last ID as `before` for
the next page. Invalid filters return 400.

Each entry uses snake_case keys: `id`, `occurred_at`, `client_ip`, `domain`, `type`,
`rcode`, `duration`, `source`, `upstream`, `cache_hit`. Time is UTC RFC3339; duration
is nanoseconds, matching other duration fields. The UI renders it in milliseconds.

The Query log page supports all four filters and older pages. History includes private
network activity: restrict access to the management interface and database backups.
