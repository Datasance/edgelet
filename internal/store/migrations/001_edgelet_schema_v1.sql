-- Edgelet schema v1 (semicolon-free: one statement per blank-line-separated block)

CREATE TABLE IF NOT EXISTS controller_microservices (
    uuid                TEXT    PRIMARY KEY,
    image_name          TEXT    NOT NULL DEFAULT '',
    container_id        TEXT    NOT NULL DEFAULT '',
    registry_id         INTEGER NOT NULL DEFAULT 0,
    rebuild             INTEGER NOT NULL DEFAULT 0,
    host_network_mode   INTEGER NOT NULL DEFAULT 0,
    is_privileged       INTEGER NOT NULL DEFAULT 0,
    log_size            INTEGER NOT NULL DEFAULT 0,
    is_router           INTEGER NOT NULL DEFAULT 0,
    exec_enabled        INTEGER NOT NULL DEFAULT 0,
    microservice_name   TEXT    NOT NULL DEFAULT '',
    application_name    TEXT    NOT NULL DEFAULT '',
    is_nats             INTEGER NOT NULL DEFAULT 0,
    schedule            INTEGER NOT NULL DEFAULT 0,
    delete_flag         INTEGER NOT NULL DEFAULT 0,
    delete_with_cleanup INTEGER NOT NULL DEFAULT 0,
    is_stuck_in_restart INTEGER NOT NULL DEFAULT 0,
    is_updating         INTEGER NOT NULL DEFAULT 0,
    config              TEXT,
    run_as_user         TEXT,
    platform            TEXT,
    runtime             TEXT,
    container_ip        TEXT,
    annotations         TEXT,
    pid_mode            TEXT,
    ipc_mode            TEXT,
    cpu_set_cpus        TEXT,
    memory_limit        INTEGER,
    port_mappings       TEXT    NOT NULL DEFAULT '[]',
    volume_mappings     TEXT    NOT NULL DEFAULT '[]',
    env_vars            TEXT    NOT NULL DEFAULT '[]',
    args                TEXT    NOT NULL DEFAULT '[]',
    cdi_devs            TEXT    NOT NULL DEFAULT '[]',
    cap_add             TEXT    NOT NULL DEFAULT '[]',
    cap_drop            TEXT    NOT NULL DEFAULT '[]',
    extra_hosts         TEXT    NOT NULL DEFAULT '[]',
    healthcheck         TEXT,
    updated_at          INTEGER NOT NULL DEFAULT (strftime('%s','now'))
)

CREATE TABLE IF NOT EXISTS controller_registries (
    id          INTEGER PRIMARY KEY,
    url         TEXT    NOT NULL DEFAULT '',
    is_public   INTEGER NOT NULL DEFAULT 0,
    user_name   TEXT    NOT NULL DEFAULT '',
    password    TEXT    NOT NULL DEFAULT '',
    user_email  TEXT    NOT NULL DEFAULT '',
    updated_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
)

CREATE TABLE IF NOT EXISTS controller_volume_mounts (
    uuid          TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    version       REAL NOT NULL DEFAULT 0,
    kind          TEXT NOT NULL CHECK(kind IN ('SECRET','CONFIGMAP')),
    checksum      TEXT NOT NULL DEFAULT '',
    microservices TEXT NOT NULL DEFAULT '[]',
    data          TEXT NOT NULL DEFAULT '{}',
    updated_at    INTEGER NOT NULL DEFAULT (strftime('%s','now'))
)

CREATE TABLE IF NOT EXISTS runtime_container_refs (
    ms_uuid     TEXT NOT NULL,
    scope       TEXT NOT NULL CHECK(scope IN ('controller','local')),
    workload_id TEXT NOT NULL DEFAULT '',
    sandbox_id  TEXT NOT NULL DEFAULT '',
    updated_at  INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    PRIMARY KEY (ms_uuid, scope)
)

CREATE INDEX IF NOT EXISTS idx_runtime_container_refs_workload ON runtime_container_refs(workload_id)

CREATE TABLE IF NOT EXISTS agent_edgeguard_signature (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    signature_jwt TEXT NOT NULL,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
)

CREATE TABLE IF NOT EXISTS agent_credentials (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    private_key_b64 TEXT NOT NULL,
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
)

CREATE TABLE IF NOT EXISTS local_service_account_tokens (
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
)

CREATE UNIQUE INDEX IF NOT EXISTS idx_local_service_account_tokens_jti ON local_service_account_tokens(jti)

CREATE INDEX IF NOT EXISTS idx_local_service_account_tokens_active_expiry ON local_service_account_tokens(expires_at, revoked_at)

CREATE INDEX IF NOT EXISTS idx_local_service_account_tokens_microservice ON local_service_account_tokens(microservice_uuid)

CREATE TABLE IF NOT EXISTS local_workloads (
  local_uuid TEXT PRIMARY KEY,
  application_name TEXT NOT NULL DEFAULT '',
  microservice_name TEXT NOT NULL DEFAULT '',
  source_name TEXT NOT NULL DEFAULT '',
  manifest_yaml TEXT NOT NULL,
  image_name TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'unknown',
  container_id TEXT NOT NULL DEFAULT '',
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
)

CREATE INDEX IF NOT EXISTS idx_local_workloads_state ON local_workloads(state)

CREATE INDEX IF NOT EXISTS idx_local_workloads_app_name ON local_workloads(application_name, microservice_name)

CREATE INDEX IF NOT EXISTS idx_local_workloads_desired_runtime ON local_workloads(desired_state, runtime_state)

CREATE INDEX IF NOT EXISTS idx_local_workloads_name_active ON local_workloads(application_name, microservice_name, deleted_at)

CREATE INDEX IF NOT EXISTS idx_local_workloads_runtime_reconcile ON local_workloads(desired_state, runtime_state, failure_count)

CREATE UNIQUE INDEX IF NOT EXISTS idx_local_workloads_unique_app_name ON local_workloads(application_name COLLATE NOCASE, microservice_name COLLATE NOCASE) WHERE COALESCE(deleted_at, 0) = 0

CREATE TABLE IF NOT EXISTS local_registries (
  id INTEGER PRIMARY KEY,
  url TEXT NOT NULL DEFAULT '',
  is_public INTEGER NOT NULL DEFAULT 0,
  user_name TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  user_email TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
)

CREATE TABLE IF NOT EXISTS local_runtime_classes (
  name TEXT PRIMARY KEY,
  handler TEXT NOT NULL,
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
  updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
)

CREATE UNIQUE INDEX IF NOT EXISTS idx_local_runtime_classes_handler ON local_runtime_classes(handler)

CREATE TABLE IF NOT EXISTS system_control_plane (
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
)
