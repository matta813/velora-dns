CREATE TABLE cluster_state (
  id INTEGER PRIMARY KEY CHECK(id = 1),
  role TEXT NOT NULL CHECK(role IN ('standalone','primary','replica')),
  cluster_id TEXT NOT NULL DEFAULT '',
  node_id TEXT NOT NULL,
  node_name TEXT NOT NULL DEFAULT '',
  advertised_url TEXT NOT NULL DEFAULT '',
  primary_url TEXT NOT NULL DEFAULT '',
  node_secret TEXT NOT NULL DEFAULT '',
  allow_insecure INTEGER NOT NULL DEFAULT 0 CHECK(allow_insecure IN (0,1)),
  created_at TEXT NOT NULL DEFAULT '',
  last_sync_at TEXT NOT NULL DEFAULT '',
  last_sync_error TEXT NOT NULL DEFAULT '',
  applied_revision TEXT NOT NULL DEFAULT ''
);
CREATE TABLE cluster_members (
  node_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  address TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  credential_hash TEXT NOT NULL,
  joined_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL DEFAULT '',
  applied_revision TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT ''
);
CREATE TABLE cluster_join_tokens (
  token_hash TEXT PRIMARY KEY,
  expires_at TEXT NOT NULL
);
