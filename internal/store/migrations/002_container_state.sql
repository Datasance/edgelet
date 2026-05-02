-- container_state: maps microservice UUID to workload and sandbox container IDs.
-- Used for fast lookup (DB-first) with label-based fallback.
CREATE TABLE IF NOT EXISTS container_state (
    ms_uuid     TEXT PRIMARY KEY,
    workload_id TEXT NOT NULL DEFAULT '',
    sandbox_id  TEXT NOT NULL DEFAULT '',
    updated_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
