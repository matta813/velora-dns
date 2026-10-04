ALTER TABLE audit_events ADD COLUMN role TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN target TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN result TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_events ADD COLUMN status_code INTEGER NOT NULL DEFAULT 0;
CREATE INDEX audit_events_retention ON audit_events(occurred_at);
