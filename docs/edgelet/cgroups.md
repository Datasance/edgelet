# Edgelet cgroups (embedded engine)

Cgroup bootstrap for `containerEngine: edgelet` only. Docker and Podman keep using the host engine cgroup configuration.

## Overview

At daemon start edgelet:

1. Detects cgroup **v1**, **v2**, or **hybrid** layout (prefers unified v2; warns on hybrid).
2. Selects **systemd** or **cgroupfs** driver for embedded containerd/crun.
3. On **cgroupfs** hosts only: creates an agent subtree and moves the daemon into it.
4. Writes `SystemdCgroup` and (when cgroupfs) `cgroup.path` into generated containerd config (overridable via `config.d/*.toml`).

## Driver selection

| Condition | Driver | `SystemdCgroup` | `cgroup.path` in containerd config |
|-----------|--------|-----------------|-------------------------------------|
| systemd service, host root has **cpuset**, not nested | systemd | `false` | omitted (daemon stays in `edgelet.service`) |
| Nested container, non-systemd init, or no root cpuset | cgroupfs | `false` | `/edgelet/agent/containerd` |

The **driver** gate  (`INVOCATION_ID` set **and** root exposes `cpuset`). **`cgroupDriver=systemd`** means host integration via `edgelet.service` and `Delegate=yes`; it does **not** enable crun's systemd cgroup backend. Edgelet always sets **`SystemdCgroup=false`** for crun.

## systemd unit

`edgelet.service` uses `Delegate=yes` (not a custom slice or explicit controller list). The edgelet daemon and embedded containerd child inherit that delegated unit; crun creates pod cgroups under it via the cgroupfs backend.

## Agent subtree (cgroupfs only)

When the cgroupfs driver is selected (nested containers, missing root cpuset, non-systemd init):

- Agent: `/edgelet/agent`
- containerd child: `/edgelet/agent/containerd`

Do **not** set `SystemdCgroup=true` for crun — it fails with systemd D-Bus or BPF errors when creating pod sandboxes. Do **not** combine `cgroup.path` with bare-metal systemd driver selection.

## Preflight vs bootstrap

| Stage | Command / hook | Checks | Mutates cgroups |
|-------|----------------|--------|-----------------|
| **Light preflight** | `edgelet cgroup-preflight` in init `start_pre` | cgroup fs mounted; v1/v2/hybrid detectable | No |
| **Bootstrap** | `runtime-bootstrap` / daemon start | Delegation prep + agent subtree | Yes |
| **Strict validate** | After prep in fat runtime | `cpu`, `memory`, `pids` available for cgroupfs workload or machine-root hosts | No |

## Machine-root hosts (LXC/VM)

Some lightweight VMs (OrbStack Alpine with OpenRC, WSL2) expose a machine boundary cgroup such as **`/.lxc`** instead of a bare `/` root. These are **not** workload-nested containers — `cgroupNested` stays `false` in status.

OpenRC installs register **`edgelet-cgroup-prep`** at **sysinit** when `/sys/fs/cgroup/.lxc` exists. It reparents processes and enables `cgroup.subtree_control` on the unified root and `/.lxc` (Moby/dind-style) before `edgelet-containerd` starts.

Fat **`runtime-bootstrap`** treats **`/.lxc/init`** and other **`/.lxc/*`** staging paths as machine-root boundaries (not workload-nested). When delegation is already satisfied at the unified root and `/.lxc`, bootstrap **skips reparent** and prepares the **current service cgroup** (e.g. `openrc.edgelet-containerd`) and **`/edgelet`** for the agent subtree before spawning embedded containerd — matching the prep that previously only ran on the accidental nested path.

systemd distros (cloud VMs, OrbStack Ubuntu) use `Delegate=yes` / host integration instead — no `edgelet-cgroup-prep` unit.

## Workload-nested edgelet container

Development deploys of the scratch image inside Docker are supported when the container is started with **`--privileged`**:

```bash
docker run -d --name edgelet --privileged \
  -v /var/lib/edgelet:/var/lib/edgelet \
  -v /etc/edgelet:/etc/edgelet \
  ghcr.io/eclipse-iofog/edgelet:<tag>
```

Datasance mirror: `ghcr.io/datasance/edgelet:<tag>`

Without `--privileged`, cgroup controller delegation fails and edgelet exits or CRI returns errors such as `controller cpu is not available`.

**Bootstrap (nested only):** before creating the agent subtree, edgelet runs prep on the container cgroup root and on `/edgelet` — evacuate processes to init, enable `cgroup.subtree_control` from available controllers. Hybrid v1 and bare-metal hosts skip this prep. **`--cgroupns=host` is not required.**

## Microservice limits

Per-microservice fields enforced on the edgelet engine (CRI/crun):

- `memoryLimit` (bytes)
- `cpuSetCpus`

Node configuration `memoryLimit` / `cpuLimit` remain **monitor-only** (not cgroup-enforced).

Missing **hugetlb** / **rdma** controllers on edge hardware are tolerated.

## Status

`edgelet system status -o json` includes (embedded edgelet engine):

| Key | Meaning |
|-----|---------|
| `cgroupMode` | `v1`, `v2`, or `hybrid` |
| `cgroupDriver` | `systemd` or `cgroupfs` |
| `cgroupNested` | `true` when running inside a **workload** container (Docker/k8s dev image), not LXC/VM machine roots |
| `cgroupDelegatedControllers` | Comma-separated delegated v2 controllers |
| `cgroupAgentPath` | Logical agent subtree path (cgroupfs mode) |
| `cgroupContainerdPath` | Logical containerd path (written to config only in cgroupfs mode) |

## Related docs

- [troubleshooting.md](troubleshooting.md) — cgroup delegation errors
- [container-engine.md](container-engine.md) — engine selection
- [deployment.md](deployment.md) — production deployment
