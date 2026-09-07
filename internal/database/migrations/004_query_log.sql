CREATE TABLE query_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  occurred_at TEXT NOT NULL,
  client_ip TEXT NOT NULL,
  domain TEXT NOT NULL,
  query_type TEXT NOT NULL,
  response_code TEXT NOT NULL,
  duration_micros INTEGER NOT NULL,
  source TEXT NOT NULL,
  upstream TEXT NOT NULL DEFAULT '',
  cache_hit INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX query_log_time_idx ON query_log(occurred_at DESC);
CREATE INDEX query_log_domain_idx ON query_log(domain, occurred_at DESC);
CREATE INDEX query_log_client_idx ON query_log(client_ip, occurred_at DESC);
