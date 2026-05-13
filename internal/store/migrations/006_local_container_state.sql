CREATE TABLE IF NOT EXISTS local_container_state (
  ms_uuid TEXT PRIMARY KEY,
  workload_id TEXT NOT NULL,
  sandbox_id TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE INDEX IF NOT EXISTS idx_local_container_state_workload
  ON local_container_state(workload_id);
