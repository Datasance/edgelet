-- Schema version tracking: prevents re-running migrations on reboot
CREATE TABLE IF NOT EXISTS schema_versions (
    version     INTEGER PRIMARY KEY,
    description TEXT    NOT NULL,
    applied_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- Microservices: exact columns matching models.Microservice
-- Scalar/nullable primitives -> real typed columns
-- Arrays and nested structs -> JSON TEXT columns
CREATE TABLE IF NOT EXISTS microservices (
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
);

-- Registries: exact columns matching models.Registry (all fields are scalar)
CREATE TABLE IF NOT EXISTS registries (
    id          INTEGER PRIMARY KEY,
    url         TEXT    NOT NULL DEFAULT '',
    is_public   INTEGER NOT NULL DEFAULT 0,
    user_name   TEXT    NOT NULL DEFAULT '',
    password    TEXT    NOT NULL DEFAULT '',
    user_email  TEXT    NOT NULL DEFAULT '',
    updated_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- Volume mounts: scalar index fields as real columns, dynamic content as JSON
-- microservices = JSON array of microservice UUIDs using this volume mount
-- data = JSON object of key->base64 pairs (the actual volume content)
CREATE TABLE IF NOT EXISTS volume_mounts (
    uuid          TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    version       REAL NOT NULL DEFAULT 0,
    kind          TEXT NOT NULL CHECK(kind IN ('SECRET','CONFIGMAP')),
    checksum      TEXT NOT NULL DEFAULT '',
    microservices TEXT NOT NULL DEFAULT '[]',
    data          TEXT NOT NULL DEFAULT '{}',
    updated_at    INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
