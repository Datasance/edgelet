# Packaging Structure

## Overview

The ioFog Agent Go implementation uses **tarball-only** distribution with two **flavors** per architecture:

| Flavor | Daemon | `containerEngine` in config | Notes |
|--------|--------|------------------------------|--------|
| **full** (default) | CGO=1, embedded containerd | `iofog` only | `dockerUrl` must be `unix:///run/iofog-agent/containerd.sock` |
| **lite** | CGO=0 | `docker` or `podman` only | External socket; Podman uses `podman.socket` in systemd units |

DEB/RPM packages are not used for this product line. Installation, upgrade, and rollback are handled by **`install.sh`**; removal by **`uninstall.sh`**.

---

## Distribution artifacts

Release tarball names:

`iofog-agent-<VERSION>-linux-<ARCH>[-musl]-<flavor>.tar.gz`

Examples:

- `iofog-agent-v3.0.0-linux-amd64-full.tar.gz`
- `iofog-agent-v3.0.0-linux-arm64-lite.tar.gz`

Each tarball contains:

- `iofog-agent` — CLI (build metadata includes **flavor**)
- `iofog-agentd` — Daemon
- Optional `config.yaml.sample` from packaging

**Checksum manifests** (produced by `make release-tarballs`):

- `build/release/SHA256SUMS-lite`
- `build/release/SHA256SUMS-full`

---

## Build

```bash
# Default local build: full flavor CLI + daemon
make build

# Explicit flavors
make build-daemon-lite
make build-daemon-full

# All Linux release arches (lite + full binaries per arch)
make build-all-archs

# Tarballs + per-flavor SHA256SUMS-* under build/release/
make release-tarballs VERSION=v1.2.3
```

Embedded dependency download (for full / CGO builds):

```bash
make deps ARCH=amd64
```

---

## install.sh

Default: **`--flavor=full`**.

| Flag | Meaning |
|------|---------|
| `--flavor=full\|lite` | Must match the tarball |
| `--airgap` | Do not download; requires `--tarball-path` |
| `--tarball-path=PATH` | Local `.tar.gz` (also accepts legacy `--bin-path`) |
| `--expected-sha256` / `--checksum-path` | Optional verification for airgap |
| `--upgrade` | Writes `previous-release`, installs new version |
| `--rollback` | Restores from `previous-release` + optional local tarball |

Metadata (**POSIX key=value** files, no JSON or extra interpreters):

**`/var/backups/iofog-agent/install-receipt`** (written on successful install / upgrade / rollback):

```text
installed_version=v1.2.3
flavor=full
source_url=https://...   # or file:///absolute/path/to.tar.gz
```

**`/var/backups/iofog-agent/previous-release`** (written at start of `--upgrade`):

```text
previous_version=v1.2.2
previous_flavor=full
previous_download_url=https://...
config_backup_path=/var/backups/iofog-agent/config.yaml.20250101120000
```

Each value is a single line; `=` is allowed in values (everything after the first `=` on the line).

**Flavor change (lite ↔ full)** on the same host is **not** supported; uninstall and reinstall with the correct tarball.

### systemd

- **Docker**: `After=` / `Wants=docker.service`
- **Podman**: `After=` / `Wants=podman.socket`
- **full** flavor: hardened unit (`ProtectSystem=strict`, `ReadWritePaths=` including `/var/lib/iofog-agent-containerd`, `/run/iofog-agent`, etc.)

Other init systems (OpenRC, SysV, s6, runit, upstart) keep the previous minimal service templates.

---

## Example configs

| File | Use |
|------|-----|
| [config_lite.yaml](iofog-agent/etc/iofog-agent/config_lite.yaml) | Lite / docker or podman |
| [config_full.yaml](iofog-agent/etc/iofog-agent/config_full.yaml) | Full / iofog |

---

## uninstall.sh

Stops the service, removes unit files and binaries. Use `--remove-data` to drop data directories (`/var/lib/iofog-agent`, containerd store, logs, etc.). Backup metadata under `/var/backups/iofog-agent` may be removed manually if desired.

---

## CI

GitHub Actions workflow **`.github/workflows/agent-go.yml`** builds **lite + full** for `linux/amd64`, `arm64`, `arm`, and `riscv64` and runs unit tests. All configured targets must pass for release gating.
