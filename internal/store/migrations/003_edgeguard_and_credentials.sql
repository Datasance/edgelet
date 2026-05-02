CREATE TABLE IF NOT EXISTS edgeguard_signature (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    signature_jwt TEXT NOT NULL,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE TABLE IF NOT EXISTS agent_credentials (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    private_key_b64 TEXT NOT NULL,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
