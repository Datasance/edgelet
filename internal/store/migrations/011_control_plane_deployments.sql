-- Plan 12: singleton ControlPlane deployment record (at most one row per Edgelet)

CREATE TABLE IF NOT EXISTS control_plane_deployments (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    controller_uuid TEXT NOT NULL,
    namespace TEXT NOT NULL DEFAULT 'default',
    name TEXT NOT NULL DEFAULT '',
    manifest_yaml TEXT NOT NULL,
    image TEXT NOT NULL DEFAULT '',
    container_id TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'unknown',
    desired_state TEXT NOT NULL DEFAULT 'running',
    runtime_state TEXT NOT NULL DEFAULT 'unknown',
    last_error TEXT NOT NULL DEFAULT '',
    restart_count INTEGER NOT NULL DEFAULT 0,
    last_transition_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    last_reconcile_at INTEGER NOT NULL DEFAULT 0,
    last_start_attempt_at INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,
    deleted_at INTEGER,
    generation INTEGER NOT NULL DEFAULT 1,
    observed_generation INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
