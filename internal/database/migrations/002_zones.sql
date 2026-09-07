CREATE TABLE zones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    primary_ns TEXT NOT NULL,
    contact TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK(revision > 0)
);
CREATE TABLE zone_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    zone_id INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    ttl INTEGER NOT NULL CHECK(ttl BETWEEN 0 AND 86400),
    value TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0 CHECK(priority BETWEEN 0 AND 65535)
);
CREATE INDEX zone_records_zone ON zone_records(zone_id);
