# Engine lifecycle integration tests (Plan 9A-5)

## Status

**Plan 9A-5 complete** (2026-06-02). Run on **macOS + Lima**:

```bash
./test/engine-lifecycle/run-all.sh
```
> **Spec:** [.cursor/edgelet/docs/09a-container-engine-lifecycle.md](../../.cursor/edgelet/docs/09a-container-engine-lifecycle.md) § Phase 9A-5  
> **Contract:** [.cursor/edgelet/ENGINE-LIFECYCLE.md](../../.cursor/edgelet/ENGINE-LIFECYCLE.md)

End-to-end integration tests for **cold `containerEngine` switches** (edgelet ↔ docker) inside a Lima Linux VM with **Docker preinstalled**. Complements `test/embedded/` (edgelet engine only).

---

## Directory layout

```
test/engine-lifecycle/
├── README.md                      # this file
├── run-all.sh                     # setup → build → VM → install → test
├── setup.sh                       # macOS prerequisites (reuse embedded toolchain)
├── build.sh                       # unified linux edgelet binary (embed tar for edgelet leg)
├── lima-ubuntu-docker.yaml        # Lima VM: Ubuntu 24.04 + docker.io + edgelet deps
├── vm-start.sh                    # create/start Lima VM from lima-ubuntu-docker.yaml
├── vm-stop.sh                     # stop / delete VM
├── vm-install.sh                  # copy edgelet into VM; --start-engine=edgelet|docker
├── vm-setup-inside.sh             # in-VM install (runs via vm-install.sh)
├── engine-switch-test.sh          # main assertions (runs inside VM via lima shell)
├── docker-url-reload-test.sh      # optional: warm dockerUrl reload (same engine)
├── fixtures/
│   └── engine-switch-ms.yaml      # single MS manifest for deploy step
└── lib/
    └── log.sh                     # source ../embedded/lib/log.sh
```

**Default Lima VM name:** `edgelet-engine-lifecycle` (separate from `test/embedded/` `iofog-test`).

---

## Test flow (engine-switch-test.sh)

Each switch case follows the same six steps:

| Step | Action | Assert |
|------|--------|--------|
| 1 | Install edgelet with **start engine A** (`edgelet` or `docker`) | `runtime.engineReady`, deploy succeeds |
| 2 | `edgelet deploy -f fixtures/engine-switch-ms.yaml` | MS listed as running |
| 3 | Change engine: `edgelet config --ce B` (or API PATCH) | `runtime.pendingRestart == true`; reconcile quiesced |
| 4 | Before service restart | MS **containers removed**; spec row still in DB |
| 5 | `systemctl restart edgelet` | Daemon up with engine B |
| 6 | After restart | MS **recreated from DB** on engine B; `pendingRestart == false` |

**Required matrix (plan DoD):**

| Case | Start engine | Switch to |
|------|--------------|-----------|
| `edgelet-to-docker` | `edgelet` | `docker` |
| `docker-to-edgelet` | `docker` | `edgelet` |

**Optional:** `docker-url-reload-test.sh` — same `containerEngine: docker`, change `dockerUrl` only; no restart; container stays reachable.

---

## Lima VM (lima-ubuntu-docker.yaml)

Based on `test/embedded/lima-ubuntu.yaml`, with these differences:

- Install and enable **Docker** (`docker.io`, `docker.service` active)
- Do **not** disable Docker when testing docker engine
- Disable **host containerd** only (avoid clash with edgelet's private socket)
- Slightly larger disk/RAM (embed extract + Docker images)
- Same `vzNAT`, port-forward ignore, cgroup-friendly kernel modules as embedded IT

---

## Usage

```bash
# Full pipeline (macOS + Lima)
./test/engine-lifecycle/run-all.sh

# Single switch direction
./test/engine-lifecycle/run-all.sh --switch=edgelet-to-docker
./test/engine-lifecycle/run-all.sh --switch=docker-to-edgelet

# Iteration (VM already running)
./test/engine-lifecycle/run-all.sh --skip-setup --skip-build --skip-start
./test/engine-lifecycle/engine-switch-test.sh --vm-name=edgelet-engine-lifecycle --switch=edgelet-to-docker
```

---

## Relationship to other tests

| Suite | Engine | Purpose |
|-------|--------|---------|
| `test/embedded/` | `edgelet` only | Embedded containerd, CNI, RuntimeClass, chaos |
| `test/engine-lifecycle/` | `edgelet` **and** `docker` | Cold engine switch + MS recreate |
| `test/embedded/container-deploy-smoke.sh` | nested edgelet in Docker | Plan 9B cgroup smoke (different gate) |

 `./test/engine-lifecycle/run-all.sh` passes both required switch cases.
