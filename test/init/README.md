# Plan 10 — Init systems integration tests

> **Spec:** [.cursor/edgelet/docs/10-init-systems-enterprise.md](../../.cursor/edgelet/docs/10-init-systems-enterprise.md) §Phase 10-6  
> **Skill:** `@edgelet-plan-10-init-systems`

## Test matrix

| ID | Script / artifact | Environment | Pass criteria |
|----|-------------------|-------------|---------------|
| **T10-A** | `systemd-install-smoke.sh` | Lima `iofog-test` or linux `--native` | install + `edgelet.service` active; DelegateSubgroup; cgroup status OK |
| **T10-B** | `vm-start-alpine.sh` + `alpine-openrc-smoke.sh` | Lima `edgelet-openrc` | openrc install + start/stop (**init**) |
| **T10-B+** | `alpine-openrc-runtime-smoke.sh` (Plan **10-9**) | Same VM, after **10-8** | static fat; API + socket; one MS deploy; restart stable |
| **T10-C** | `rhel-sysv-checklist.md` | RHEL sysv manual | Checklist sign-off |
| **T10-D** | `openwrt-procd-checklist.md` (Plan **10-10**) | OpenWrt + procd manual | Install, procd start/stop, preflight |
| **T10-E** | tier-2 (optional) | Debian sysv | Best effort — sysv template + install.sh |

## Runner

```bash
go test ./internal/cgroups/...   # regression (also run from run-all.sh)

./test/init/run-all.sh
./test/init/run-all.sh --case=systemd
./test/init/run-all.sh --case=openrc
```

`run-all.sh` with `--case=all` or `--case=openrc` calls **`vm-start-alpine.sh`** automatically (creates/starts `edgelet-openrc` from `lima-alpine-openrc.yaml`). T10-B is skipped only when `limactl` is not installed.

## T10-A (systemd)

**macOS + Lima (default):**

Smoke tests stage a minimal install tree into the VM (`install.sh`, `scripts/lib/`, `scripts/edgelet-shutdown`, `packaging/init/`) so `install.sh` resolves `SCRIPT_DIR` correctly.

```bash
./test/embedded/build.sh
./test/embedded/vm-start.sh
./test/init/systemd-install-smoke.sh
```

**Linux (native root):**

```bash
make build-linux-amd64   # or arm64
sudo ./test/init/systemd-install-smoke.sh --native
```

## T10-B (Alpine openrc)

Uses a **bootable Alpine cloud qcow2** (not minirootfs). Apple Silicon (`aarch64`) only in `lima-alpine-openrc.yaml`. Lima `containerd.system` / `containerd.user` are `false` (OpenRC has no systemd).

```bash
make build-linux-arm64
./test/init/vm-start-alpine.sh
./test/init/alpine-openrc-smoke.sh
```

Or one shot (starts VM + T10-B + T10-B+):

```bash
./test/init/run-all.sh --case=openrc
```

T10-B+ uses `--after-t10-b` from `run-all.sh` (no second `install.sh`; avoids orphaned `containerd-child`). Standalone runtime on a fresh VM:

```bash
./test/init/alpine-openrc-runtime-smoke.sh --fresh-install
```

Stop / delete the VM:

```bash
./test/init/vm-stop-alpine.sh
./test/init/vm-stop-alpine.sh --delete
```

Do **not** use `limactl create template:alpine` for T10-B — that image uses busybox as PID 1 while still shipping `openrc-run`, which breaks `rc-service`. Always use `vm-start-alpine.sh` (creates from `lima-alpine-openrc.yaml`). If an old VM exists, delete it first: `vm-stop-alpine.sh --delete`.

## Lima JSON / VM detection

`limactl list --json` on Lima 2.x is **JSONL** (one object per line). Init scripts use `test/init/lib/lima.sh` (`select(.name == $n)`), matching `test/embedded/vm-start.sh`. Do not use `.[] | select(.name == …)` — that fails with jq errors and false “VM not found” warnings.

## T10-C (RHEL sysv)

Manual gate: complete [rhel-sysv-checklist.md](rhel-sysv-checklist.md) on a lab RHEL/sysv node.

## Prerequisites

- Built `build/edgelet-linux-*`
- macOS + Lima for T10-A / T10-B host-driven tests
- `jq` on the host (for Lima list parsing) and in the Alpine VM (provision installs it)

## Plan 10 extension (10-8 … 10-10)

| Phase | IT |
|-------|-----|
| **10-8** | Static embed default; CI `file` gate on fat `edgelet` |
| **10-9** | T10-B+ runtime smoke (requires `STATIC_BUILD=true` default) |
| **10-10** | T10-D OpenWrt procd checklist + `packaging/init/procd/` |

## T10-B+ (Alpine runtime)

After a **static embed** build (`./test/embedded/build.sh` or `make build-linux-arm64`):

```bash
./test/init/vm-start-alpine.sh
./test/init/alpine-openrc-runtime-smoke.sh
```

Included in `./test/init/run-all.sh --case=openrc`.

## T10-D (OpenWrt procd)

Manual: [openwrt-procd-checklist.md](openwrt-procd-checklist.md).

## Status

Plan 10 complete on `edgelet/10-init-systems` (2026-06-02). Rebuild embed before T10-B+ if fat was previously dynamic glibc.
