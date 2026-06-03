# T10-D — OpenWrt procd manual checklist (Plan 10-10)

> **Spec:** [.cursor/edgelet/docs/10-init-systems-enterprise.md](../../.cursor/edgelet/docs/10-init-systems-enterprise.md)  
> **Template:** `packaging/init/procd/edgelet`

Lab gate for OpenWrt (or OpenWrt-derived) devices with **procd** as init. Requires **Plan 10-8** static embed in the installed bundle (musl-compatible fat `edgelet`).

## Prerequisites

- [ ] Device runs OpenWrt with `/sbin/procd` and `/etc/rc.common`
- [ ] Built static linux binary for device arch: `make build-linux-<arch>` (or release artifact)
- [ ] Network to Controller (if online provision) or airgap bundle staged

## Install

- [ ] Copy `edgelet-linux-<arch>` and `install.sh` (+ `packaging/init/` tree or use repo install on device)
- [ ] `install.sh --bin-path=… --container-engine=edgelet` (or `docker`/`podman` if applicable)
- [ ] `detect_init` reports **procd** (not openrc)
- [ ] `/etc/init.d/edgelet` installed from `packaging/init/procd/edgelet`
- [ ] `/usr/libexec/edgelet/edgelet-shutdown` present

## Static embed (10-8)

- [ ] `file /var/lib/edgelet/data/current/bin/edgelet` → **statically linked** (no `ld-linux` interpreter)
- [ ] Embedded shim/crun under extract tree also static (no musl loader missing errors in `logread`)

## procd service

- [ ] `/etc/init.d/edgelet enable`
- [ ] `/etc/init.d/edgelet start` — service **running** (`/etc/init.d/edgelet status`)
- [ ] `edgelet cgroup-preflight` succeeds (or logged in start failure)
- [ ] `pgrep -f 'edgelet daemon'` shows thin wrapper / fat child as expected

## Runtime (embedded engine)

- [ ] `test -S /run/edgelet/containerd.sock`
- [ ] `edgelet system status` returns daemon payload
- [ ] `edgelet system status -o json` → `cgroupDriver` is **cgroupfs** on non-systemd

## Stop path

- [ ] `/etc/init.d/edgelet stop` invokes **edgelet-shutdown** (no orphaned containerd children after grace)
- [ ] `/etc/init.d/edgelet start` after stop — socket and API return

## Sign-off

| Field | Value |
|-------|-------|
| Device / image | |
| OpenWrt version | |
| edgelet version / hash | |
| Tester | |
| Date | |
| Pass / fail | |
