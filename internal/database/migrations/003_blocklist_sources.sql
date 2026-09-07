CREATE TABLE blocklist_sources (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  url TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
  last_updated_at TEXT,
  last_error TEXT NOT NULL DEFAULT ''
);
CREATE TABLE blocklist_domains (
  source_id INTEGER NOT NULL REFERENCES blocklist_sources(id) ON DELETE CASCADE,
  domain TEXT NOT NULL,
  PRIMARY KEY(source_id, domain)
);
CREATE INDEX blocklist_domains_domain_idx ON blocklist_domains(domain);
