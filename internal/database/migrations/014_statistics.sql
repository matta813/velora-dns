CREATE TABLE statistics (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    format_version INTEGER NOT NULL,
    queries INTEGER NOT NULL,
    blocked INTEGER NOT NULL,
    cache_hits INTEGER NOT NULL,
    cache_misses INTEGER NOT NULL,
    rate_limit_rejections INTEGER NOT NULL,
    upstream_requests TEXT NOT NULL DEFAULT '{}',
    upstream_errors TEXT NOT NULL DEFAULT '{}',
    saved_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
