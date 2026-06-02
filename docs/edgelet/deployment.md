# Edgelet deployment

Production installation for Edgelet on **linux**, **darwin**, and **windows**. Plan 8 ships **raw release binaries** (no `.tar.gz` bundles, no DEB/RPM).

Contract reference: [.cursor/edgelet/RELEASE-INSTALL.md](../../.cursor/edgelet/RELEASE-INSTALL.md).

## Prerequisites

| Platform | Requirements |
|----------|----------------|
| **linux** | Root/sudo; Ubuntu 20.04+, RHEL 8+, Debian 11+ (or equivalent); network to Controller for online install |
| **darwin** | Admin for `/usr/local/bin`; Docker or Podman for `containerEngine` |
| **windows** | Admin; Docker or Podman |
| **edgelet engine (linux)** | No external container runtime |
| **docker / podman** | Docker 26.10+ or Podman 3.0+ on host |

## GitHub Releases (binary-only)

Artifacts per tag on **`datasance/edgelet`**:

| OS / arch | File |
|-----------|------|
| linux amd64 / arm64 / arm / riscv64 | `edgelet-linux-<arch>` |
| darwin amd64 / arm64 | `edgelet-darwin-<arch>` |
| windows amd64 | `edgelet-windows-amd64.exe` |

Also: `SHA256SUMS`, `edgelet-config.yaml.sample`, `edgelet-controller-ca.crt.sample`, `install.sh`, `uninstall.sh`.

Download URL:

```text
https://github.com/datasance/edgelet/releases/download/<tag>/edgelet-<os>-<arch>[.exe]
```

Override repo: `EDGELET_GITHUB_REPO=datasance/edgelet`.

Build locally: `make release-binaries VERSION=v1.0.0` → `dist/`.

## Installation

### Online (linux example)

```bash
curl -fsSL https://github.com/datasance/edgelet/releases/download/vX.Y.Z/install.sh -o install.sh
chmod +x install.sh
sudo ./install.sh --version=vX.Y.Z
```

Or from a cloned repo (dev / CI):

```bash
sudo ./install.sh --bin-path=build/edgelet-linux-amd64 --version=dev
```

### First-time config

If `/etc/edgelet/config.yaml` is missing, `install.sh` copies `edgelet-config.yaml.sample` (from packaging or `/usr/share/edgelet/`). You can also run:

```bash
sudo edgelet init-config
```

`init-config` is idempotent: it does **not** overwrite an existing file.

Controller CA (`/etc/edgelet/cert.crt`) is **not** installed by default. Use `--with-sample-ca` for lab installs, or install the production cert via **`edgelet provision`** / `edgelet config cert`.

### Install flags

| Flag | Purpose |
|------|---------|
| `--version=` | Release tag (default `latest` when downloading) |
| `--arch=` | Override auto-detected arch |
| `--bin-path=` | Local binary (dev, airgap, CI) |
| `--airgap` | Offline; requires `--bin-path` |
| `--expected-sha256=` | Verify binary (airgap / supply-chain) |
| `--checksum-path=` | `SHA256SUMS` file for verification |
| `--upgrade` / `--rollback` | Thin-binary OTA |
| `--force-config` | Replace config from sample (destructive) |
| `--with-sample-ca` | Copy sample controller CA if `cert.crt` missing |
| `--container-engine=` | `edgelet`, `docker`, or `podman` (linux default: **edgelet**) |

**Removed (Plan 8):** `--flavor`, `--provision-key`, `--non-interactive`, `--tarball-path`.

### Install paths

| Item | Linux | macOS | Windows |
|------|-------|-------|---------|
| Binary | `/usr/local/bin/edgelet` | `/usr/local/bin/edgelet` | `Program Files\Edgelet\edgelet.exe` |
| Config | `/etc/edgelet/config.yaml` | `/etc/edgelet/config.yaml` (or `~/.config/edgelet/` for dev) | `%ProgramData%\Edgelet\config.yaml` |
| Controller CA | `/etc/edgelet/cert.crt` | same | same |
| Data | `/var/lib/edgelet/` | `/var/lib/edgelet/` | `%ProgramData%\Edgelet\data\` |
| OTA metadata | `/var/backups/edgelet/` | — | — |
| Bundled scripts | `/usr/share/edgelet/install.sh` | optional | optional |

Linux init: **systemd**, **procd** (OpenWrt), OpenRC, SysV, upstart, s6, runit (auto-detected). Templates: `packaging/init/`.

### Portable embedded build (Plan 10-8)

The fat runtime inside the zstd embed tar is **statically linked by default** (`STATIC_BUILD=true` in `scripts/build-edgelet` / `make deps`). That lets `containerEngine=edgelet` run on **glibc and musl** (Alpine, OpenWrt) without a separate `-musl` release artifact.

| Build | Command |
|-------|---------|
| Default (static fat + embed gate) | `make build-linux-arm64` or `./scripts/build-edgelet fat` after `deps` |
| Fast local iteration (dynamic fat) | `STATIC_BUILD=false make build-linux-amd64` |

CI and packaging run `scripts/check-embed-static.sh` on `build/bin/edgelet` and `build/stage/bin/` after `package-data`.

## potctl / iofogctl contract

Orchestrators must **not** pass provision keys to `install.sh`.

1. SSH to the edge host.
2. **Online:** run `install.sh` on the remote (downloads binary from GitHub).
3. **Airgap:** download `edgelet-linux-<arch>` + verify `SHA256SUMS` on the orchestrator → SCP to remote →  
   `install.sh --airgap --bin-path=… --expected-sha256=…`
4. **Provision:** SSH `edgelet provision <key>` (and `edgelet config …` as needed).

Example airgap on device:

```bash
sudo ./install.sh \
  --airgap \
  --bin-path=/tmp/edgelet-linux-arm64 \
  --expected-sha256=<sha256-from-SHA256SUMS> \
  --version=v1.2.3
```

## Daemon and systemd

Production linux uses the **thin** binary as the service entry point:

```bash
edgelet daemon          # extract fat bundle if needed → exec runtime
systemctl start edgelet # ExecStart=/usr/local/bin/edgelet daemon
```

Unit templates: `packaging/init/systemd/edgelet.service`.

For **docker** or **podman**, add a drop-in `After=docker.service` (or podman). The daemon retries socket connection with backoff before staying in WARNING.

### Embedded runtime layout (`containerEngine: edgelet`)

| Path | Role |
|------|------|
| `/var/lib/edgelet/data/<hash>/` | Content-addressed fat bundle extract |
| `data/current` | Symlink to active hash |
| `data/previous` | Last `current` before rotation (manual / coordinated rollback) |

Operator CLI (`edgelet ms`, `edgelet deploy`, …) runs in the **thin** process and does not trigger extract.

## Two-layer OTA

### Layer 1 — Thin binary (`install.sh`)

| Action | Mechanism |
|--------|-----------|
| Upgrade | `install.sh --upgrade [--version=]` — caches previous binary, writes `previous-release` |
| Rollback | `install.sh --rollback` — restores from cache or `previous_download_url` |
| Receipt | `/var/backups/edgelet/install-receipt` |
| Cache | `/var/backups/edgelet/cache/edgelet-<version>-<os>-<arch>` |

Controller OTA: Pot `changeVersion` → field agent → detached  
`sh /usr/share/edgelet/install.sh --upgrade|--rollback`. Heartbeat exposes `readyToUpgrade` / `readyToRollback`.

**Forbidden:** `iofog-agentvc.jar`, apt/dnf/yum edgelet packages.

### Layer 2 — Fat embed (daemon)

On thin upgrade with a new embed hash, `edgelet daemon` extracts to `/var/lib/edgelet/data/<new-hash>/` and rotates `current` / `previous`.

Coordinated rollback: controller triggers `install.sh --rollback` + service restart; operators may reuse an on-disk hash tree if still present.

## Runtime engine selection

```yaml
# /etc/edgelet/config.yaml
currentProfile: production
profiles:
  production:
    containerEngine: edgelet   # linux default
    dockerUrl: unix:///run/edgelet/containerd.sock
    arch: auto
```

| `containerEngine` | Linux extract? | Host requirement |
|-------------------|----------------|------------------|
| **edgelet** | Yes (first daemon start) | None |
| `docker` | No | Docker socket |
| `podman` | No | Podman socket |

Verify:

```bash
edgelet --version
edgelet system version -o json | jq '{embeddedEngine, allowedEngines, containerEngine}'
```

## EdgeletAPI PKI

Daemon creates `/etc/edgelet/edgelet-api` and TLS material on first start. CLI uses bearer token + `edgeletapi-ca.crt`.

## Provisioning

```bash
edgelet provision <provisioning-key>
edgelet system status
edgelet deprovision
```

## Container image (linux)

Scratch image: `ghcr.io/datasance/edgelet-linux:<tag>`. OTA = orchestrator replaces image tag (`EDGELET_DAEMON=container`). See `Dockerfile.edgelet-linux`

## Uninstall

```bash
sudo sh uninstall.sh
sudo sh uninstall.sh --remove-data   # includes /etc/edgelet when set
```

## Integration tests

```bash
sudo ./test/install/install-fresh-linux.sh
sudo ./test/install/install-upgrade-rollback.sh
sudo ./test/install/install-airgap.sh
./test/embedded/run-all.sh --ci --arch=arm64   # Lima VM on macOS
```

## See also

- [architecture.md](architecture.md) — thin/fat model
- [container-engine.md](container-engine.md) — engine matrix
- [troubleshooting.md](troubleshooting.md) — service teardown
- [packaging/PACKAGING-STRUCTURE.md](../../packaging/PACKAGING-STRUCTURE.md)
