# Plan 11 — Workload continuity integration tests

> **Spec:** [.cursor/edgelet/docs/11-workload-continuity.md](../../.cursor/edgelet/docs/11-workload-continuity.md)  
> **IT consolidation (Plan 11-7):** same spec doc, Phase 11-7

## Test matrix

| ID | Script | When | Pass criteria |
|----|--------|------|---------------|
| **T11-A** | `docker-restart.sh` | After 11-1 | `restart edgelet` → same Docker container IDs; MS running |
| **T11-B** | (regression) | After 11-1 | `test/engine-lifecycle/run-all.sh` still green |
| **T11-C** | `embedded-restart.sh` | After 11-4 | restart **edgelet** only → CRI containers survive |
| **T11-D** | `embedded-runtime-restart.sh` | After 11-4 | restart containerd unit; MS down then up |
| **T11-E** | doc | Pre-11-4 | Monolithic embedded restart still drains MS |

## Runner

```bash
./test/workload-continuity/run-all.sh
./test/workload-continuity/run-all.sh --case=docker-restart
./test/workload-continuity/run-all.sh --case=embedded-restart
./test/workload-continuity/run-all.sh --skip-build    # after test/embedded/build.sh
./test/workload-continuity/run-all.sh --skip-setup     # lima/lima deps already installed
```

`run-all.sh` builds **once** (`test/embedded/build.sh`), then:

1. **T11-A/B:** `engine-lifecycle/setup.sh` → `vm-start` → docker engine + MS deploy → tests
2. **T11-C/D:** `embedded/vm-start` → `install.sh` with Plan 11 split units + MS deploy → tests (strict — no skip)

## Install path (Plan 11-7)

Embedded cases use the **production install path** shared with embedded IT:

- `test/lima/lib/install-split.sh` → `install.sh --container-engine=edgelet`
- Split units: `edgelet-containerd.service` + `edgelet.service`
- No manual bpf/crun prep in suite scripts

## Embedded split gate (T11-C/D)

All of:

- `systemctl is-active edgelet-containerd`
- `systemctl is-active edgelet`
- `/etc/systemd/system/edgelet-containerd.service` present
- At least one container in `ctr -n k8s.io containers list`

## Lima VMs

| VM | Purpose |
|----|---------|
| `edgelet-engine-lifecycle` | T11-A, T11-B (docker + engine switch) |
| `iofog-test` | T11-C, T11-D (embedded split via `install.sh`) |

## Related suites (Plan 11-7)

This directory owns **T11-A–D** only. Other regression suites:

| Suite | Runner | VM |
|-------|--------|-----|
| embedded (cgroups v2) | `../embedded/run-all.sh` | `iofog-test` |
| **embedded-cgroup-v1** | `../embedded/run-all-cgroup-v1.sh` | `iofog-test-v1` |
| engine-lifecycle | `../engine-lifecycle/run-all.sh` | `edgelet-engine-lifecycle` |
| init | `../init/run-all.sh` | `iofog-test`, Alpine OpenRC |

Post–step 7: top-level `./test/run-all.sh --suite=workload-continuity` (and `--suite=embedded-cgroup-v1`, etc.).

```bash
./test/run-all.sh --suite=workload-continuity
./test/run-all.sh --suite=embedded
./test/run-all.sh --suite=embedded-cgroup-v1
./test/run-all.sh --suite=unit
```
