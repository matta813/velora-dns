CREATE TABLE client_policies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  client_id INTEGER NOT NULL UNIQUE REFERENCES clients(id) ON DELETE CASCADE,
  mode TEXT NOT NULL CHECK(mode IN ('default','disabled','custom')),
  blocklist_ids TEXT NOT NULL DEFAULT '',
  allow_domains TEXT NOT NULL DEFAULT '',
  block_domains TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1))
);
