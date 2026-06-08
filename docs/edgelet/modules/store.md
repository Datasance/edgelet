# SQLite Store

The Store module owns the on-disk **SQLite database** (`edgelet.db`) under the configured disk directory. It provides typed accessors for Controller cache, local deploy state, service account tokens, runtime container references, and agent credentials. Migrations are embedded and applied idempotently at open.

**Code:** `internal/store/`

Operator backup/wipe guidance: [../persistence.md](../persistence.md). This document focuses on schema and module boundaries.

## Purpose

- Single-writer SQLite with WAL mode for daemon durability
- Versioned schema via embedded SQL migrations
- CRUD for controller-sourced and locally deployed entities
- Integrity check on open; WAL checkpoint on close

## Dependencies

| Depends on | Reason |
|------------|--------|
| Filesystem | `{diskDirectory}/edgelet.db` |
| `modernc.org/sqlite` | Pure Go SQLite driver |

| Used by | Reason |
|---------|--------|
| `supervisor` | Opens/closes DB around module lifetime |
| `fieldagent` | Controller cache, credentials, Edge Guard |
| `processmanager` | Local workloads, runtime refs, control plane row |
| `runtimeapi` / EdgeletAPI | Deploy apply, auth tokens, provision |
| `edgeguard` | Attestation signature row |
| `serviceaccount` | Projected token persistence |

## Lifecycle

### Open

`store.GetInstance().Open(dir)` — called once at Supervisor start:

1. `MkdirAll` disk directory (`0700`)
2. Open SQLite with `_journal_mode=WAL`, `_foreign_keys=ON`
3. `SetMaxOpenConns(1)` — single writer
4. `migrate()` — apply pending embedded migrations
5. `PRAGMA integrity_check`

### Close

`Close()` — WAL checkpoint `TRUNCATE`, then connection close. Invoked last in Supervisor shutdown.

## Schema v1

Current migration: `migrations/001_edgelet_schema_v1.sql`. `schema_versions` tracks applied version; **wipe-only** upgrades from pre-v1 agents — no in-place legacy migration.

### Controller cache (Field Agent writers)

| Table | Purpose |
|-------|---------|
| `controller_microservices` | Desired MS from Controller |
| `controller_registries` | Registry auth |
| `controller_volume_mounts` | Secrets/configmaps |

### Agent identity

| Table | Purpose |
|-------|---------|
| `agent_credentials` | Singleton Ed25519 private key (`id=1`) |
| `agent_edgeguard_signature` | Last Edge Guard attestation JWT |

### Local operator state (EdgeletAPI writers)

| Table | Purpose |
|-------|---------|
| `local_workloads` | CLI/applied Microservice manifests |
| `local_registries` | Local registry credentials |
| `local_runtime_classes` | RuntimeClass handler map |
| `system_control_plane` | Singleton ControlPlane deployment |
| `local_service_account_tokens` | Issued SA JWT metadata |

### Runtime linkage

| Table | Purpose |
|-------|---------|
| `runtime_container_refs` | Maps MS UUID + scope → containerd workload/sandbox IDs |

## Access patterns

Store exposes methods on `*DB` split by domain file:

| File | Operations |
|------|------------|
| `microservices.go` | Save/load/clear controller microservices |
| `registries.go` | Controller registries |
| `volumes.go` | Volume mount upsert/replace |
| `local_deployed_microservices.go` | Local workload CRUD |
| `local_registries.go` | Local registry CRUD |
| `local_runtime_classes.go` | RuntimeClass CRUD |
| `control_plane_deployments.go` | ControlPlane singleton |
| `service_account_tokens.go` | SA token upsert/revoke/list |
| `edgeguard_credentials.go` | Edge Guard signature |
| `runtime_container_refs` (in schema) | Accessed from processmanager/runtime paths |

Handlers should not use `Conn()` directly except in tests; prefer typed store methods to keep SQL centralized.

## Configuration

| Key | Effect |
|-----|--------|
| `diskDirectory` | Directory containing `edgelet.db` (default under `/var/lib/edgelet/`) |

## External APIs

No network surface. Data reaches operators via EdgeletAPI (`GET /v1/ms`, deploy list routes) and CLI.

## Observability

- Log module name: `"SQLite Store"`
- Migration apply logs at `INFO` per version
- Integrity failure fails daemon start loudly

## Failure modes

| Symptom | Typical cause |
|---------|----------------|
| Daemon won't start | Migration error, integrity check failure |
| Empty MS list after provision | Field Agent not synced; check controller tables |
| Duplicate local MS name | Unique index on `(application_name, microservice_name)` |
| Token revoke ineffective | Row still active until `revoked_at` set |

Restore procedure: [../persistence.md](../persistence.md).

## Code map

| File | Role |
|------|------|
| `db.go` | Singleton, open/close, integrity |
| `schema.go` | Migration runner |
| `migrations/*.sql` | Embedded DDL |
| `*_test.go`, `schema_v1_contract_test.go` | Contract tests |

Related: [fieldagent.md](fieldagent.md), [processmanager.md](processmanager.md), [edgeletapi.md](edgeletapi.md).
