CREATE TABLE dns_rewrites (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK(type IN ('A', 'AAAA', 'CNAME')),
  value TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
  description TEXT NOT NULL DEFAULT '',
  UNIQUE(name, type, value)
);
CREATE INDEX dns_rewrites_name ON dns_rewrites(name);
