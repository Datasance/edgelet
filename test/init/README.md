# Init systems integration tests

Operator guide: [docs/edgelet/init-systems.md](../../docs/edgelet/init-systems.md)

## Test matrix

| Case | Script / artifact | Environment | Pass criteria |
|------|-------------------|-------------|---------------|
| **systemd** | `systemd-install-smoke.sh` | Lima `iofog-test` or linux `--native` | install + `edgelet.service` active; DelegateSubgroup; cgroup status OK |
| **alpine-openrc** | `vm-start-alpine.sh` + `alpine-openrc-smoke.sh` | Lima `edgelet-openrc` | openrc install + start/stop (**init**) |
| **alpine-openrc-runtime** | `alpine-openrc-runtime-smoke.sh` | Same VM, static embed | static fat; API + socket; one MS deploy; restart stable |
| **rhel-sysv** | `rhel-sysv-checklist.md` | RHEL sysv manual | Checklist sign-off |
| **openwrt-procd** | `openwrt-procd-checklist.md` | OpenWrt + procd manual | Install, procd start/stop, preflight |
| **tier-2 (optional)** | sysv template | Debian sysv | Best effort — sysv template + install.sh |

## Runner

```bash
go test ./internal/cgroups/...   # regression (also run from run-all.sh)

./test/init/run-all.sh
./test/init/run-all.sh --case=systemd
./test/init/run-all.sh --case=openrc
```

`run-all.sh` with `--case=all` or `--case=openrc` calls **`vm-start-alpine.sh`** automatically (creates/starts `edgelet-openrc` from `lima-alpine-openrc.yaml`). Alpine openrc is skipped only when `limactl` is not installed.

## systemd install smoke

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

## Alpine openrc

Uses a **bootable Alpine cloud qcow2** (not minirootfs). Apple Silicon (`aarch64`) only in `lima-alpine-openrc.yaml`. Lima `containerd.system` / `containerd.user` are `false` (OpenRC has no systemd).

```bash
make build-linux-arm64
./test/init/vm-start-alpine.sh
./test/init/alpine-openrc-smoke.sh
```

Or one shot (starts VM + openrc init + runtime smoke):

```bash
./test/init/run-all.sh --case=openrc
```

Runtime smoke uses `--after-t10-b` from `run-all.sh` (no second `install.sh`; avoids orphaned `containerd-child`). Standalone runtime on a fresh VM:

```bash
./test/init/alpine-openrc-runtime-smoke.sh --fresh-install
```

Stop / delete the VM:

```bash
./test/init/vm-stop-alpine.sh
./test/init/vm-stop-alpine.sh --delete
```

Do **not** use `limactl create template:alpine` for Alpine openrc — that image uses busybox as PID 1 while still shipping `openrc-run`, which breaks `rc-service`. Always use `vm-start-alpine.sh` (creates from `lima-alpine-openrc.yaml`). If an old VM exists, delete it first: `vm-stop-alpine.sh --delete`.

## Lima JSON / VM detection

`limactl list --json` on Lima 2.x is **JSONL** (one object per line). Init scripts use `test/init/lib/lima.sh` (`select(.name == $n)`), matching `test/embedded/vm-start.sh`. Do not use `.[] | select(.name == …)` — that fails with jq errors and false “VM not found” warnings.

## RHEL sysv manual gate

Complete [rhel-sysv-checklist.md](rhel-sysv-checklist.md) on a lab RHEL/sysv node.

## Prerequisites

- Built `build/edgelet-linux-*`
- macOS + Lima for systemd / Alpine openrc host-driven tests
- `jq` on the host (for Lima list parsing) and in the Alpine VM (provision installs it)

## Static embed + OpenWrt procd

| Topic | IT |
|-------|-----|
| Static embed default | CI `file` gate on fat `edgelet` |
| Alpine runtime smoke | Requires `STATIC_BUILD=true` default |
| OpenWrt procd | [openwrt-procd-checklist.md](openwrt-procd-checklist.md) + `packaging/init/procd/` |

## Alpine runtime smoke

After a **static embed** build (`./test/embedded/build.sh` or `make build-linux-arm64`):

```bash
./test/init/vm-start-alpine.sh
./test/init/alpine-openrc-runtime-smoke.sh
```

Included in `./test/init/run-all.sh --case=openrc`.

Rebuild embed before Alpine runtime smoke if fat was previously dynamic glibc.
