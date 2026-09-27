CREATE TABLE webhooks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE,
  url TEXT NOT NULL,
  event_types TEXT NOT NULL DEFAULT '',
  min_severity TEXT NOT NULL DEFAULT 'info' CHECK(min_severity IN ('info','warning','critical')),
  allow_private INTEGER NOT NULL DEFAULT 0 CHECK(allow_private IN (0,1)),
  token TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1))
);
