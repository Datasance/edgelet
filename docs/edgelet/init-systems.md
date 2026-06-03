# Edgelet init systems (operator guide)

> **Status:** Implemented (Plan 10)  
> **Spec:** [.cursor/edgelet/docs/10-init-systems-enterprise.md](../../.cursor/edgelet/docs/10-init-systems-enterprise.md)  
> **Contract:** [.cursor/edgelet/INIT-SYSTEMS.md](../../.cursor/edgelet/INIT-SYSTEMS.md)

Production-grade Linux init integration: ordering, cgroup delegation, engine dependencies, and a shared control-plane stop path.

---

## Support tiers

| Tier | Init systems | SLA |
|------|--------------|-----|
| **Tier 1 (production)** | **systemd**, **openrc** (Alpine/Gentoo), **procd** (OpenWrt) | Full IT (`test/init/`) or manual checklist (T10-D) |
| **Tier 2 (best effort)** | sysvinit, s6, runit, upstart | Hardened templates; same preflight/shutdown helpers; documented limits |

---

## Distro × init × engine matrix

| Distro family | Typical init | Tier | `containerEngine=edgelet` | `docker` / `podman` |
|---------------|--------------|------|---------------------------|---------------------|
| Ubuntu / Debian / RHEL / Fedora | systemd | 1 | Recommended (DelegateSubgroup) | Drop-in orders after engine unit |
| Alpine / Gentoo | openrc | 1 | Allowed (cgroupfs; **static embed** required on musl) | `need docker` / `need podman` in `depend()` |
| OpenWrt | procd | 1 | Allowed (static embed; cgroupfs) | Operator wires docker/podman separately |
| RHEL legacy / old appliances | sysvinit | 2 | Allowed; cgroupfs driver | No engine unit ordering in LSB script |
| Container / minimal | s6, runit | 2 | Allowed | Operator wires engine separately |
| Ubuntu 14–16 era | upstart | 2 | Allowed | `pre-start` preflight only |

**Production SLA:** prefer **systemd** for embedded (`edgelet`) so cgroup v2 delegation uses `DelegateSubgroup`. Non-systemd embedded is supported (Plan 9B cgroupfs) but documented as best-effort for tier 2.

---

## Canonical packaging layout

| Item | Path |
|------|------|
| systemd control unit | `packaging/init/systemd/edgelet.service` |
| systemd data-plane stub | `packaging/init/systemd/edgelet-containerd.service` (Plan 11) |
| Engine drop-ins | `packaging/init/systemd/edgelet.service.d/{docker,podman}.conf` |
| openrc / procd / sysv / s6 / runit / upstart | `packaging/init/{openrc,procd,sysvinit,s6,runit,upstart}/` |
| Shutdown helper | `scripts/edgelet-shutdown` → `/usr/libexec/edgelet/edgelet-shutdown` |
| Shipped templates on node | `/usr/share/edgelet/init/` |

Install uses **`packaging/init/` only** — there is no `packaging/systemd/` install path (CI-guarded).

---

## Shared helpers

| Helper | Command / path | When |
|--------|----------------|------|
| **Preflight** | `edgelet cgroup-preflight` | `start_pre` (openrc), sysv/s6/runit/upstart start, before daemon |
| **Shutdown** | `/usr/libexec/edgelet/edgelet-shutdown` → `edgelet shutdown` | systemd `ExecStop`, all init `stop` paths |

Preflight on the **thin** `/usr/local/bin/edgelet` uses a procfs/cgroupfs-only probe (`DetectPreflight`); full cgroup subtree setup stays in the **fat** runtime (`Detect` / `Bootstrap` with `containerd/cgroups`).

`edgelet shutdown` tries EdgeletAPI graceful stop, then SIGTERM/SIGKILL fallback. **Drain / leave-running policy** is owned by Plan 11; Plan 10 only defines the stop entry.

`TimeoutStopSec=120` on systemd equals default `shutdownGracePeriodSeconds` (90) + 30s buffer (Plan 11). Embedded engine uses `edgelet.service.d/edgelet.conf` drop-in with `EDGELET_RUNTIME_SPLIT=1`.

---

## systemd (Tier 1)

| Setting | Value |
|---------|--------|
| `Delegate` | `yes` |
| `DelegateSubgroup` | `supervisor` (avoids cgroup v2 EBUSY on restart) |
| `KillMode` | `process` |
| `ExecStop` | `/usr/libexec/edgelet/edgelet-shutdown` |
| Engine ordering | Install selects `edgelet.service.d/docker.conf` or `podman.conf` — not `sed` on the base unit |

```bash
systemctl cat edgelet
systemctl show edgelet -p DelegateSubgroup,TimeoutStopSec
```

After Plan 11: enable `edgelet-containerd.service` before `edgelet.service` for embedded split.

**Monolithic embedded:** the control unit omits `ProtectSystem=strict` until the data-plane unit owns containerd (embedded needs `/etc/cni`, `/run`, `/opt`, etc.). `init-edgelet.sh` creates `/etc/cni/net.d`, `/run/edgelet`, and `/run/containerd` before `systemctl start`.

---

## openrc (Tier 1)

- `depend()`: `net` + optional `docker` / `podman` (install-time)
- `start_pre`: `edgelet cgroup-preflight`
- `stop`: `edgelet-shutdown`
- Stub: `/etc/init.d/edgelet-containerd` (Plan 11 chain)

```bash
rc-service edgelet start
rc-service edgelet stop
```

### Logging (openrc)

| Log | Source |
|-----|--------|
| `/var/log/edgelet/daemon.log` | OpenRC `output_log` / `error_log` — thin wrapper and pre-exec errors |
| `/var/log/edgelet/edgelet.0.log` (rotated) | Fat daemon after `edgelet daemon` starts (`logDiskDirectory`) |

---

## procd (Tier 1 — OpenWrt)

- Template: `packaging/init/procd/edgelet` (`USE_PROCD=1`)
- `start_service`: `edgelet cgroup-preflight` then `edgelet daemon` under procd respawn
- `stop_service`: `/usr/libexec/edgelet/edgelet-shutdown`
- Detection: **procd before openrc** when `/sbin/procd` and `/etc/rc.common` exist

```bash
/etc/init.d/edgelet enable
/etc/init.d/edgelet start
/etc/init.d/edgelet stop
```

On some images the binary lives at `/usr/sbin/edgelet`; install.sh still defaults to `/usr/local/bin/edgelet` — adjust paths in the template if your image requires it.

Manual gate: [test/init/openwrt-procd-checklist.md](../../test/init/openwrt-procd-checklist.md) (T10-D).

---

## Portable embedded engine (Plan 10-8)

Fat `edgelet` in the embed tar is **statically linked by default** so musl hosts (Alpine, OpenWrt) can `exec` the runtime without glibc `ld-linux`. Build with `make build-linux-<arch>`; opt out via `STATIC_BUILD=false` for faster local builds only.

---

## Tier 2 limits

| Topic | Tier 2 behavior |
|-------|-----------------|
| `DelegateSubgroup` | **Not available** — no systemd delegation |
| Embedded cgroup driver | **cgroupfs** on non-systemd (Plan 9B) |
| Engine ordering | Manual / site-specific (except openrc tier 1) |
| MS survival on control restart | Document monolithic behavior until Plan 11 split |

All tier-2 templates call the same **`edgelet-shutdown`** helper as systemd.

---

## Install

```bash
sudo ./install.sh --bin-path=build/edgelet-linux-amd64 --container-engine=docker
```

Init detection: `scripts/lib/init-detect.sh` (OpenRC when the supervisor is active — `rc-status` or `/etc/inittab` — not merely when `openrc-run` exists; Alpine links `/sbin/init` to busybox). Unit install: `scripts/lib/init-edgelet.sh`.

---

## Integration tests

See [test/init/README.md](../../test/init/README.md): T10-A (systemd), T10-B (Alpine openrc init), T10-B+ (Alpine runtime), T10-C (RHEL sysv), T10-D (OpenWrt procd).

---

## Related docs

- [cgroups.md](cgroups.md) — Plan 9B driver detection (unchanged by Plan 10)
- [workload-continuity.md](workload-continuity.md) — Plan 11 control/data plane split
- [deployment.md](deployment.md) — install paths
