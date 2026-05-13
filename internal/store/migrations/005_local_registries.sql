-- Dedicated persistence for local (non-controller managed) registries

CREATE TABLE IF NOT EXISTS local_registries (
  id INTEGER PRIMARY KEY,
  url TEXT NOT NULL DEFAULT '',
  is_public INTEGER NOT NULL DEFAULT 0,
  user_name TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  user_email TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
