CREATE TABLE cluster_state (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    cluster_id TEXT NOT NULL UNIQUE,
    node_id TEXT NOT NULL UNIQUE,
    node_name TEXT NOT NULL,
    control_address TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('leader', 'voter')),
    ca_certificate BLOB NOT NULL,
    certificate BLOB NOT NULL,
    private_key BLOB NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE cluster_join_tokens (
    token_hash BLOB PRIMARY KEY,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX cluster_join_tokens_expiry ON cluster_join_tokens(expires_at);
