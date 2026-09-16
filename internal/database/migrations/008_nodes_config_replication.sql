-- Nodes table for cluster membership
CREATE TABLE nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    address TEXT NOT NULL,
    capabilities TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unknown',
    last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Config versions table for config replication
CREATE TABLE config_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version INTEGER NOT NULL,
    config_hash TEXT NOT NULL,
    applied_by TEXT NOT NULL DEFAULT '',
    applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX config_versions_version_idx ON config_versions(version DESC);
