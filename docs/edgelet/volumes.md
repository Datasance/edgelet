# Workload volumes and lifecycle

How Edgelet stores microservice volume data on disk, what is removed when a workload is deleted, and how operators reclaim space.

Applies to all container engines; **embedded `edgelet` engine** behavior is called out where it differs from Docker/Podman.

---

## Why this matters

On the embedded `edgelet` engine, a `VOLUME` mapping does **not** use Docker's volume subsystem. Edgelet creates a persistent directory under `{diskDirectory}/volumes/data/` and bind-mounts it into the container.

**Deleting a microservice removes the container, but usually not the volume data.** Orphaned data is reclaimed by **volume prune** (scheduled or manual), not at delete time.

Operators who need immediate cleanup must run prune or delete the directories manually.

---

## On-disk layout

Base path: `{diskDirectory}` (default `/var/lib/edgelet/`).

```
volumes/
  data/{microservice-uuid}/{volume-name}/     # VOLUME-type persistent data (edgelet engine)
  microservices/{microservice-uuid}/          # Per-MS mount staging (secrets/configmaps)
  secrets/{volume-name}/                      # Controller secret payloads (shared)
  configMaps/{volume-name}/                   # Controller configmap payloads (shared)
```

Image layers and container filesystems live under `/var/lib/edgelet-containerd/` (embedded engine), **not** under `volumes/data/`. Backing up `edgelet.db` alone does **not** back up `VOLUME` data — see [persistence.md](persistence.md).

---

## Volume mapping types

| Type | `hostDestination` | Edgelet behavior |
|------|-------------------|------------------|
| **`VOLUME`** | Relative name, e.g. `mydata` | Creates `{diskDirectory}/volumes/data/{ms-uuid}/mydata/` and bind-mounts it |
| **`VOLUME_MOUNT`** | Controller volume name, e.g. `my-secret` or `my-secret/key` | Materializes under `volumes/microservices/{ms-uuid}/` from shared `secrets/` or `configMaps/` trees |
| **`BIND`** | Absolute host path, e.g. `/opt/data` | Bind-mounts that path; Edgelet never deletes it |

Local deploy manifests (`edgelet deploy -f`) support **`BIND`** and **`VOLUME`** only. Controller workloads may also use **`VOLUME_MOUNT`** for secrets and configmaps synced from Pot.

---

## What happens when a microservice is deleted

Deletion paths:

- **Controller:** microservice marked `delete: true` in Pot snapshot → process manager reconciles removal
- **Local:** `edgelet ms rm <id>` or EdgeletAPI delete of a local workload

In all cases Edgelet:

1. Stops and removes the container
2. Runs volume-mount cleanup for that microservice UUID

### Always removed

| Path | When |
|------|------|
| Container / CRI sandbox | On delete |
| `volumes/microservices/{uuid}/` | On delete (per-MS secret/configmap symlinks) |

### Not removed on delete

| Path | Reason |
|------|--------|
| `volumes/data/{uuid}/` | **VOLUME** data is intentionally persistent until prune |
| `volumes/secrets/`, `volumes/configMaps/` | Shared controller artifacts; other workloads may still reference them |
| **`BIND`** host paths | Operator-managed; Edgelet does not touch them |

---

## `deleteWithCleanup` (controller workloads)

When Pot sets `deleteWithCleanup: true` on a microservice:

| Engine | Effect |
|--------|--------|
| **docker / podman** | Container remove may pass `RemoveVolumes` to the engine; also removes the image when cleanup is requested |
| **edgelet (embedded)** | **`deleteWithCleanup` does not delete `volumes/data/`** — the engine ignores the cleanup flag for volumes. The main extra effect is **image removal** when cleanup is requested |

Local `edgelet ms rm` always uses non-cleanup removal (container only; no automatic image or volume data delete).

---

## When volume data is actually deleted

### Volume prune (embedded engine)

`PruneVolumes` removes orphaned directories under:

- `volumes/data/{uuid}/`
- `volumes/microservices/{uuid}/`

for any microservice UUID that has **no running container**.

Triggered by:

- Scheduled prune (`pruningFrequency` / `frequencyInterval` in config) — runs containers → volumes → images
- Manual prune:

```bash
edgelet system prune volumes
edgelet system prune all
```

### Full deprovision (`scope=all`)

Agent deprovision stops all workloads, then runs container and volume prune hooks. This is the broadest automatic cleanup short of wiping `diskDirectory`.

### Controller artifact clear (`scope=local` deprovision)

Clears `secrets/` and `configMaps/` and SQLite volume-mount rows, but **preserves** `volumes/data/` (local workload data).

### Manual cleanup

If you must reclaim space immediately after deleting a microservice:

```bash
# After confirming the MS UUID is gone and data is not needed
sudo rm -rf /var/lib/edgelet/volumes/data/<microservice-uuid>
```

Or run `edgelet system prune volumes` to remove all orphaned volume trees.

---

## Operator checklist

| Goal | Action |
|------|--------|
| Delete workload, keep data for redeploy | `edgelet ms rm` or controller delete — data remains under `volumes/data/{uuid}/` |
| Delete workload and reclaim disk soon | `edgelet system prune volumes` or wait for scheduled prune |
| Backup stateful workloads | Copy `edgelet.db` **and** `volumes/data/` (and BIND paths if used) |
| Wipe node completely | Stop services, remove `diskDirectory` (see [persistence.md](persistence.md)) |
| Secret/configmap only used by one MS | Shared trees are not auto-deleted on MS delete; controller deprovision or manual removal |

---

## Troubleshooting

### Disk usage grows after microservices are deleted

1. List remaining volume data:

   ```bash
   sudo du -sh /var/lib/edgelet/volumes/data/*
   ```

2. Confirm no container still references the UUID:

   ```bash
   edgelet ms ls
   ```

3. Prune orphans:

   ```bash
   edgelet system prune volumes
   ```

### Redeployed microservice has empty volume

`VOLUME` data is keyed by **microservice UUID**. A new deployment with a new UUID gets a **new** directory under `volumes/data/`. To preserve data across UUID changes, use **`BIND`** to a fixed host path you control.

---

## Related documentation

| Document | Topic |
|----------|--------|
| [manifest-reference.md](manifest-reference.md) | `spec.container.volumes` YAML fields |
| [persistence.md](persistence.md) | SQLite backup; include `volumes/data/` for stateful apps |
| [container-engine.md](container-engine.md) | `diskDirectory` vs containerd paths |
| [modules/volumemount.md](modules/volumemount.md) | Implementation details (Volume Mount Manager) |
| [modules/pruning.md](modules/pruning.md) | Pruning manager and scheduled prune order |
