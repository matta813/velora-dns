ALTER TABLE blocklist_sources ADD COLUMN update_interval_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE blocklist_sources ADD COLUMN last_attempt_at TEXT;
ALTER TABLE blocklist_sources ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;
