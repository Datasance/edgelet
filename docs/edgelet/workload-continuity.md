# Edgelet workload continuity (operator guide)

> **Spec:** [.cursor/edgelet/docs/11-workload-continuity.md](../../.cursor/edgelet/docs/11-workload-continuity.md)  
> **Contract:** [.cursor/edgelet/WORKLOAD-CONTINUITY.md](../../.cursor/edgelet/WORKLOAD-CONTINUITY.md)

## Principle

Restarting **`edgelet`** (control plane) must **not** stop microservice containers unless you restart the **runtime** unit or perform a **cold engine change** (Plan 9A).

| Layer | systemd unit | Owns |
|-------|--------------|------|
| Data plane | `edgelet-containerd.service` | embedded containerd, CRI socket, MS containers |
| Control plane | `edgelet.service` | supervisor, field agent, Edgelet API, reconcile |

**docker / podman:** the host engine **is** the data plane — edgelet does not stop MS on control restart.

---

## docker / podman (leave-running)

Default **`shutdownPolicy=leave-running`** for external engines.

| Action | MS containers |
|--------|---------------|
| `systemctl restart edgelet` | **Keep running** — same container ID |
| `edgelet shutdown` (control stop) | **No drain** — reconcile reattaches on start |

Config (`/etc/edgelet/config.yaml`):

```yaml
shutdownPolicy: leave-running   # default for docker/podman
shutdownGracePeriodSeconds: 90  # used for optional maintenance / data-plane stops
```

Verify after control restart:

```bash
docker ps --filter label=iofog.org/microservice-uuid
edgelet system status -o json | jq '."runtime.agentPhase", ."runtime.shutdownPolicy"'
```

Reattach uses **labels + DB** — not Docker `RestartPolicy`.

---

## embedded engine (runtime split)

After Plan 11-4, production embedded installs use two units:

```bash
systemctl restart edgelet              # control only — MS survive (T11-C)
systemctl restart edgelet-containerd   # data plane — MS stop then reconcile (T11-D)
```

Cgroup bootstrap (**C1**) runs in `edgelet runtime-bootstrap` on the containerd unit. Control unit **attach-only** (`EDGELET_RUNTIME_SPLIT=1`).

**systemd coupling:** `edgelet-containerd.service` is **not** `PartOf=edgelet.service`. Stopping or restarting **only** `edgelet` leaves the data plane running (T11-C).

**Full embedded shutdown** (backup, uninstall, wipe):

```bash
sudo systemctl stop edgelet-containerd.service edgelet.service
```

---

## Before runtime split (monolithic embedded)

Until **`edgelet-containerd.service`** is active with runtime split, a monolithic `systemctl restart edgelet` still **drains MS** (`shutdownPolicy=drain-all` default). Expect brief MS outage (Q12b).

---

## Status during control restart

- Brief MS **UNKNOWN** on the controller dashboard is OK.
- Local status exposes **`runtime.agentPhase=restarting`** during control stop/start.
- Field-agent status POST **annotates** MS entries with `controlRestart: true` — posts are not suppressed.

---

## Cold engine change (unchanged — Plan 9A)

Changing **`containerEngine`** still requires quiesce, MS cleanup, `pendingRestart`, service restart, and recreate from DB. Plan 11 does **not** relax this path.

---

## OTA restart matrix

| Bundle change | Restart |
|---------------|---------|
| Thin **edgelet** binary only | `systemctl restart edgelet` |
| Fat / containerd runtime bundle | `systemctl restart edgelet-containerd` then `edgelet` |

See [deployment.md](deployment.md) and install docs for hash-based OTA details.

---

## Integration tests

```bash
./test/workload-continuity/run-all.sh
./test/workload-continuity/run-all.sh --case=docker-restart
./test/workload-continuity/run-all.sh --case=embedded-restart
```

### Suite matrix (Plan 11-7)

| Suite | Runner | VM |
|-------|--------|-----|
| workload-continuity | `test/workload-continuity/run-all.sh` | `edgelet-engine-lifecycle`, `iofog-test` |
| embedded (v2) | `test/embedded/run-all.sh` | `iofog-test` |
| embedded-cgroup-v1 | `test/embedded/run-all-cgroup-v1.sh` | `iofog-test-v1` (hybrid v1) |

Full IT consolidation plan: [.cursor/edgelet/docs/11-workload-continuity.md](../../.cursor/edgelet/docs/11-workload-continuity.md) (Phase 11-7).
