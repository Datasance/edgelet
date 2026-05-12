-- LocalAPI v3 token metadata and local deployment persistence

CREATE TABLE IF NOT EXISTS service_account_tokens (
  id TEXT PRIMARY KEY,
  token_use TEXT NOT NULL,
  principal_type TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL,
  microservice_uuid TEXT NOT NULL DEFAULT '',
  application_name TEXT NOT NULL DEFAULT '',
  service_account_name TEXT NOT NULL DEFAULT '',
  role_ref_kind TEXT NOT NULL DEFAULT '',
  role_ref_name TEXT NOT NULL DEFAULT '',
  rbac_version TEXT NOT NULL DEFAULT 'v1',
  rules_by_group_json TEXT NOT NULL DEFAULT '{}',
  claims_json TEXT NOT NULL DEFAULT '{}',
  issuer TEXT NOT NULL DEFAULT '',
  audience TEXT NOT NULL DEFAULT '',
  alg TEXT NOT NULL DEFAULT '',
  jti TEXT NOT NULL,
  token_sha256 TEXT NOT NULL,
  issued_at INTEGER NOT NULL,
  not_before INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  revoked_at INTEGER,
  rotated_from_jti TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
  updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_service_account_tokens_jti
  ON service_account_tokens(jti);

CREATE INDEX IF NOT EXISTS idx_service_account_tokens_active_expiry
  ON service_account_tokens(expires_at, revoked_at);

CREATE INDEX IF NOT EXISTS idx_service_account_tokens_microservice
  ON service_account_tokens(microservice_uuid);

CREATE TABLE IF NOT EXISTS local_deployed_microservices (
  local_uuid TEXT PRIMARY KEY,
  application_name TEXT NOT NULL DEFAULT '',
  microservice_name TEXT NOT NULL DEFAULT '',
  source_name TEXT NOT NULL DEFAULT '',
  manifest_yaml TEXT NOT NULL,
  image_name TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'unknown',
  container_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
  updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE INDEX IF NOT EXISTS idx_local_deployed_microservices_state
  ON local_deployed_microservices(state);

CREATE INDEX IF NOT EXISTS idx_local_deployed_microservices_app_name
  ON local_deployed_microservices(application_name, microservice_name);
