-- RuntimeClass persistence for local API v3 runtime extension

CREATE TABLE IF NOT EXISTS local_runtime_classes (
  name TEXT PRIMARY KEY,
  handler TEXT NOT NULL,
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
  updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_local_runtime_classes_handler
  ON local_runtime_classes(handler);

