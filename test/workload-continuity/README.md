# Workload continuity integration tests

Operator guide: [docs/edgelet/workload-continuity.md](../../docs/edgelet/workload-continuity.md)

## Test matrix

| Case | Script | When | Pass criteria |
|------|--------|------|---------------|
| **docker-restart** | `docker-restart.sh` | leave-running policy | `restart edgelet` → same Docker container IDs; MS running |
| **engine-lifecycle regression** | (regression) | after leave-running | `test/engine-lifecycle/run-all.sh` still green |
| **embedded-restart** | `embedded-restart.sh` | runtime split active | restart **edgelet** only → CRI containers survive |
| **embedded-runtime-restart** | `embedded-runtime-restart.sh` | runtime split active | restart containerd unit; poll until CRI drain (0 ctr rows), then MS reconciles |
| **monolithic doc** | doc | pre-split | Monolithic embedded restart still drains MS |

## Runner

```bash
./test/workload-continuity/run-all.sh
./test/workload-continuity/run-all.sh --case=docker-restart
./test/workload-continuity/run-all.sh --case=embedded-restart
./test/workload-continuity/run-all.sh --skip-build    # after test/embedded/build.sh
./test/workload-continuity/run-all.sh --skip-setup     # lima/lima deps already installed
```

`run-all.sh` builds **once** (`test/embedded/build.sh`), then:

1. **docker / engine-lifecycle:** `engine-lifecycle/setup.sh` → `vm-start` → docker engine + MS deploy → tests
2. **embedded split:** `embedded/vm-start` → `install.sh` with split units + MS deploy → tests (strict — no skip)

## Install path

Embedded cases use the **production install path** shared with embedded IT:

- `test/lima/lib/install-split.sh` → `install.sh --container-engine=edgelet`
- Split units: `edgelet-containerd.service` + `edgelet.service`
- No manual bpf/crun prep in suite scripts

## Embedded split gate

All of:

- `systemctl is-active edgelet-containerd`
- `systemctl is-active edgelet`
- `/etc/systemd/system/edgelet-containerd.service` present
- At least one container in `ctr -n k8s.io containers list`

After changing `edgelet-containerd.service`, reinstall on the VM (`install.sh` or copy the unit) and run `systemctl daemon-reload` before embedded split tests.

## Lima VMs

| VM | Purpose |
|----|---------|
| `edgelet-engine-lifecycle` | docker-restart + engine-lifecycle regression |
| `iofog-test` | embedded split via `install.sh` |

## Related suites

| Suite | Runner | VM |
|-------|--------|-----|
| embedded (cgroups v2) | `../embedded/run-all.sh` | `iofog-test` |
| **embedded-cgroup-v1** | `../embedded/run-all-cgroup-v1.sh` | `iofog-test-v1` |
| engine-lifecycle | `../engine-lifecycle/run-all.sh` | `edgelet-engine-lifecycle` |
| init | `../init/run-all.sh` | `iofog-test`, Alpine OpenRC |

Top-level orchestrator:

```bash
./test/run-all.sh --suite=workload-continuity
./test/run-all.sh --suite=embedded
./test/run-all.sh --suite=embedded-cgroup-v1
./test/run-all.sh --suite=unit
```
