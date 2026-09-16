-- TSIG keys table
CREATE TABLE IF NOT EXISTS tsig_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    algorithm TEXT NOT NULL,
    secret TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Secondary zone fields
ALTER TABLE zones ADD COLUMN zone_type TEXT NOT NULL DEFAULT 'primary';
ALTER TABLE zones ADD COLUMN primary_address TEXT NOT NULL DEFAULT '';
ALTER TABLE zones ADD COLUMN transfer_tsig_key TEXT NOT NULL DEFAULT '';
ALTER TABLE zones ADD COLUMN transfer_interval INTEGER NOT NULL DEFAULT 0;
ALTER TABLE zones ADD COLUMN last_transfer_at TEXT;
ALTER TABLE zones ADD COLUMN next_refresh_at TEXT;
ALTER TABLE zones ADD COLUMN last_transfer_serial INTEGER NOT NULL DEFAULT 0;
