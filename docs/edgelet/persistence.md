# Edgelet SQLite persistence (operator guide)

Edgelet stores on-device state in a single SQLite database. This document covers **backup and restore**, **Plan 13 wipe-only upgrades**, **secrets-at-rest expectations**, and known **JSON-in-column** schema debt.

There is **no** EdgeletAPI or CLI backup command in Plan 13 — operators use filesystem copy after stopping services.

---

## Database location

| Item | Value |
|------|--------|
| Config key | `diskDirectory` (alias `dl`) in `/etc/edgelet/config.yaml` |
| Default directory | `/var/lib/edgelet/` |
| Database file | `{diskDirectory}/edgelet.db` |
| WAL sidecars | `{diskDirectory}/edgelet.db-wal`, `{diskDirectory}/edgelet.db-shm` (when WAL journal is active) |

Resolve the path on a running node:

```bash
grep diskDirectory /etc/edgelet/config.yaml
edgelet system info -o json | jq -r '.diskDirectory'
```

On open, Edgelet creates `diskDirectory` with mode **0700** if missing. SQLite runs with **WAL** journal mode (`_journal_mode=WAL`).

**Schema version:** fresh installs apply embedded migration `001_edgelet_schema_v1.sql` and record version **1** in `schema_versions`. There is no in-place upgrade from pre–Plan 13 databases (see [Wipe-only upgrade](#wipe-only-upgrade-plan-13)).

---

## What the database holds

Tables are grouped by **source prefix** (v1 schema):

| Prefix | Examples | Contents |
|--------|----------|----------|
| `controller_*` | `controller_microservices`, `controller_registries`, `controller_volume_mounts` | Pot controller snapshot (MS list, registries, volume mounts) |
| `agent_*` | `agent_credentials`, `agent_edgeguard_signature` | Agent identity and EdgeGuard material |
| `local_*` | `local_workloads`, `local_registries`, `local_service_account_tokens`, … | EdgeletAPI deploy, local registries, RBAC tokens |
| `system_*` | `system_control_plane` | Singleton ControlPlane deployment row |
| `runtime_*` | `runtime_container_refs` | CRI/Docker workload and sandbox IDs (`scope` = `controller` \| `local`) |

Image layers and containerd state live **outside** `diskDirectory` (for example `/var/lib/edgelet-containerd/` on the embedded engine). Backing up `edgelet.db` does **not** back up pulled images or running container filesystems.

---

## Backup runbook (R85)

Back up when you need to preserve controller/local desired state across reinstall, disk migration, or disaster recovery on the **same** Edgelet build (schema v1).

### Prerequisites

- Root or sudo on the edge node
- Enough disk space for a copy of `edgelet.db` and sidecars
- Maintenance window — workloads stop while services are down

### 1. Stop services

Stop Edgelet first so the daemon can checkpoint WAL and close SQLite cleanly.

```bash
sudo systemctl stop edgelet.service
```

If the node uses the **embedded** `edgelet` engine, also stop containerd:

```bash
sudo systemctl stop edgelet-containerd.service 2>/dev/null || true
```

For `containerEngine: docker` or `podman`, only `edgelet.service` is required.

Confirm nothing holds the DB open:

```bash
sudo lsof /var/lib/edgelet/edgelet.db 2>/dev/null || true
```

(Adjust the path if `diskDirectory` is non-default.)

### 2. Copy database files

Set `DISK` to your `diskDirectory`, then copy the database and any WAL sidecars:

```bash
DISK=/var/lib/edgelet
BACKUP_DIR=/root/edgelet-db-backup-$(date +%Y%m%d-%H%M%S)
sudo mkdir -p "$BACKUP_DIR"
sudo cp -a "$DISK/edgelet.db" "$BACKUP_DIR/"
[ -f "$DISK/edgelet.db-wal" ] && sudo cp -a "$DISK/edgelet.db-wal" "$BACKUP_DIR/"
[ -f "$DISK/edgelet.db-shm" ] && sudo cp -a "$DISK/edgelet.db-shm" "$BACKUP_DIR/"
sudo chmod 700 "$BACKUP_DIR"
ls -la "$BACKUP_DIR"
```

After a **graceful** `systemctl stop edgelet`, Edgelet runs `PRAGMA wal_checkpoint(TRUNCATE)` on close; often only `edgelet.db` is needed. If you copy while the daemon was **not** stopped cleanly, include `-wal` and `-shm` or the backup may be inconsistent.

### 3. Archive (optional)

```bash
sudo tar -czf "$BACKUP_DIR.tar.gz" -C "$(dirname "$BACKUP_DIR")" "$(basename "$BACKUP_DIR")"
```

Store the archive off-node. Treat it as **sensitive** (see [Threat model](#threat-model-r86)).

### 4. Start services

```bash
sudo systemctl start edgelet-containerd.service 2>/dev/null || true
sudo systemctl start edgelet.service
```

Verify:

```bash
edgelet system status -o json | jq '{state, containerEngine}'
sudo journalctl -u edgelet -n 30 --no-pager
```

---

## Restore runbook (R85)

Restore only onto a node running a **Plan 13+** build with **schema version 1**. Restoring a pre–Plan 13 backup onto a v1 binary is unsupported — use [wipe-only upgrade](#wipe-only-upgrade-plan-13) and let the controller/EdgeletAPI repopulate state instead.

### Steps

1. **Stop** `edgelet.service` and `edgelet-containerd.service` (if present), same as backup.
2. **Replace** files under `diskDirectory`:
   - Remove current DB files: `edgelet.db`, `edgelet.db-wal`, `edgelet.db-shm`
   - Copy backup files into place with ownership/mode consistent with the edgelet user (typically root on bare metal).
3. **Start** `edgelet-containerd.service` (if used), then `edgelet.service`.
4. On start, Edgelet runs `PRAGMA integrity_check` — if the file is corrupt, the **supervisor does not start** and logs an integrity error (R83).

```bash
DISK=/var/lib/edgelet
sudo systemctl stop edgelet.service edgelet-containerd.service 2>/dev/null || true
sudo rm -f "$DISK/edgelet.db" "$DISK/edgelet.db-wal" "$DISK/edgelet.db-shm"
sudo cp -a /path/to/backup/edgelet.db "$DISK/"
[ -f /path/to/backup/edgelet.db-wal ] && sudo cp -a /path/to/backup/edgelet.db-wal "$DISK/"
[ -f /path/to/backup/edgelet.db-shm ] && sudo cp -a /path/to/backup/edgelet.db-shm "$DISK/"
sudo chmod 700 "$DISK"
sudo systemctl start edgelet-containerd.service 2>/dev/null || true
sudo systemctl start edgelet.service
```

**Not covered:** HA, replication, or online hot backup. Plan 13 does not ship Litestream or a second SQL tier.

---

## Wipe-only upgrade (Plan 13)

**RFC R80:** Edgelet does **not** migrate in-place from the old incremental schema (migrations 001–011 era) to v1. Operators on dev or lab nodes that already had an `edgelet.db` from pre–Plan 13 builds must **delete** the database before the first Plan 13 binary run.

No published production fleets require a 012→013 migrator; fresh Lima VMs and wiped DBs are the integration-test baseline.

### Procedure

```bash
sudo systemctl stop edgelet.service edgelet-containerd.service 2>/dev/null || true

DISK=/var/lib/edgelet   # or your diskDirectory
sudo rm -f "$DISK/edgelet.db" "$DISK/edgelet.db-wal" "$DISK/edgelet.db-shm"

sudo systemctl start edgelet-containerd.service 2>/dev/null || true
sudo systemctl start edgelet.service
```

After wipe:

- Controller microservices and registries are **re-pushed** from Pot on the next field-agent sync.
- Local workloads and ControlPlane require **re-deploy** via EdgeletAPI/CLI if you relied on local state.
- Legacy **JSON file → SQLite** import (`MigrateJSONToSQLite`) is **removed** — do not expect old JSON caches to repopulate the DB.

Confirm schema version after start (optional, on node with `sqlite3`):

```bash
sqlite3 /var/lib/edgelet/edgelet.db 'SELECT MAX(version) FROM schema_versions;'
# expect: 1
```

---

## Runtime checks (R83, R84)

| Event | Behavior |
|-------|----------|
| **Open** | `PRAGMA integrity_check` must return `ok`; otherwise startup **fails hard** |
| **Graceful stop** | `PRAGMA wal_checkpoint(TRUNCATE)` before closing the connection (`systemctl stop edgelet`) |

If integrity fails after restore or disk errors, treat the DB as damaged: restore from a known-good backup or wipe and reconcile from controller/local deploys.

Future schema version bumps (v2+) may run an optional one-time `VACUUM` after migration apply; not used on v1-only installs.

---

## Threat model (R86)

### Sensitive data in SQLite

| Data | Tables / columns (examples) |
|------|-----------------------------|
| Registry credentials | `controller_registries.password`, `local_registries.password` |
| Agent private key | `agent_credentials.private_key_b64` |
| EdgeGuard JWT | `agent_edgeguard_signature.signature_jwt` |
| Service account material | `local_service_account_tokens` (`token_sha256`, `claims_json`, `rules_by_group_json`, …) |
| Volume / MS config blobs | JSON/text columns on controller and local tables |

Edgelet does **not** encrypt these fields at the application layer in Plan 13.

### Trust boundary

Protection relies on **edge node tenancy** and **host filesystem** controls:

- `diskDirectory` is created with mode **0700**.
- The SQLite file must not be world-readable; limit backup archives to admin access.
- A single `edgelet` process holds one DB (`store.GetInstance()` singleton).
- Physical or root access to the node implies read access to the DB and backups.

### Out of scope (Plan 13)

- SQLCipher or field-level encryption
- TPM-sealed keys or OS full-disk encryption policy (operator choice outside Edgelet)
- Centralized secrets vault sync into SQLite

**Future (regulated verticals):** app-level encryption at rest may be added in a later plan; R86 documents the current OS-boundary model only.

---

## JSON-in-column schema debt (R87)

Several v1 tables store structured data as **JSON text columns** instead of normalized child tables. Examples:

| Table | JSON / blob columns |
|-------|------------------------|
| `controller_microservices` | `port_mappings`, `volume_mappings`, `env_vars`, `args`, `annotations`, … |
| `controller_volume_mounts` | `microservices`, `data` |
| `local_service_account_tokens` | `rules_by_group_json`, `claims_json` |
| `local_workloads` | `manifest_yaml` (YAML blob) |

**Accepted for Plan 13:** behavior and Pot snapshot semantics stay unchanged; queries and migrations remain simple.

**Future work:** normalize hot paths (ports, env, RBAC rules) into relational tables with strict migrator versions — tracked as schema debt under **RFC R87**, not scheduled in Plan 13. See spec [§4 Schema v1](../../.cursor/edgelet/docs/13-persistence.md#4-schema-v1-tables) and tracker [13-persistence.md](../../.cursor/edgelet/plans/13-persistence.md).

---

## Plan 13 regression gates (Lima IT)

Integration tests assume a **fresh schema v1 database**. Wipe the VM disk or delete `edgelet.db` (+ WAL/SHM) on the Lima guest before running the gates below.

### Prerequisites

- `limactl` and Lima VMs (see each suite’s `run-all.sh` header; typical names: `iofog-test`, `edgelet-engine-lifecycle`).
- Repo root as working directory.

### Wipe DB inside a Lima VM (before IT)

```bash
# Replace INSTANCE with your VM name (e.g. iofog-test)
limactl shell INSTANCE -- sudo systemctl stop edgelet.service edgelet-containerd.service 2>/dev/null || true
limactl shell INSTANCE -- sudo rm -f /var/lib/edgelet/edgelet.db /var/lib/edgelet/edgelet.db-wal /var/lib/edgelet/edgelet.db-shm
```

For a completely clean slate, recreate or reset the Lima instance per [deployment.md](deployment.md) embedded-engine section, then run setup scripts once.

### Run gates (from repo root)

```bash
./test/control-plane/run-all.sh
./test/workload-continuity/run-all.sh
./test/embedded/run-all.sh
```

### Unit gate (no Lima)

```bash
go test ./internal/store/... ./internal/fieldagent/... ./internal/processmanager/... \
  ./internal/runtimeapi/... ./internal/edgeletapi/... ./internal/auth/... -short -count=1
```

---

## Related documentation

| Document | Topic |
|----------|--------|
| [deployment.md](deployment.md) | Install, systemd units, `diskDirectory` layout |
| [troubleshooting.md](troubleshooting.md) | Daemon won't start (includes disk space under `/var/lib/edgelet`) |
| [control-plane.md](control-plane.md) | ControlPlane redeploy after DB wipe |
| [container-engine.md](container-engine.md) | `/var/lib/edgelet` vs `edgelet-containerd` data paths |
