CREATE TABLE system_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_key TEXT NOT NULL,
    severity TEXT NOT NULL,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    link TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'admin',
    occurred_at TEXT NOT NULL,
    repeat_count INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX system_events_recent ON system_events(id DESC);
CREATE INDEX system_events_dedup ON system_events(event_key, id DESC);
CREATE TABLE system_event_reads (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id INTEGER NOT NULL REFERENCES system_events(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, event_id)
);
