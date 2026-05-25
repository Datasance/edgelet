# Packaging Structure — Edgelet

## Overview

Edgelet uses **tarball-only** distribution with two **flavors** per Linux architecture (RFC R20):

| Flavor | Build | `containerEngine` | Notes |
|--------|-------|-------------------|--------|
| **full** | `cgo,full` embedded containerd | `edgelet` | `dockerUrl` → `unix:///run/edgelet/containerd.sock` |
| **lite** | `lite`, CGO=0 | `docker` or `podman` | External engine only |

No **musl-suffixed** release line (RFC R9). Optional ops-only static linking: `STATIC_BUILD=true make build-linux-amd64`.

DEB/RPM are not used. Installation is via **`install.sh`**; removal via **`uninstall.sh`**.

---

## Distribution artifacts

Release tarball names:

`edgelet-<os>-<arch>-{full|lite}.tar.gz`

Versioned copies: `edgelet-<VERSION>-linux-<arch>-{full|lite}.tar.gz`

Examples:

- `edgelet-linux-amd64-full.tar.gz`
- `edgelet-linux-arm64-lite.tar.gz`
- `edgelet-darwin-arm64-lite.tar.gz` (lite only)

Each tarball contains:

- `edgelet` — single multicall binary (CLI + daemon + containerd child on full linux)
- Optional `config.yaml.sample`

Checksum manifests (`make release-tarballs`):

- `dist/SHA256SUMS-lite`
- `dist/SHA256SUMS-full`

---

## Build

```bash
# Local default
make build-edgelet-full    # linux full (needs deps embed on linux or Docker)
make build-edgelet-lite

# Linux matrix (no musl targets)
make build-all-archs

# Release tarballs → dist/
make release-tarballs VERSION=v1.0.0

# Embedded bundle (linux)
make deps ARCH=amd64

# macOS dev — linux CI gates in Docker
make ci-docker
# or: docker build -f build/Dockerfile.embedded -t edgelet-embed-ci .
#     docker run --rm -v "$(pwd)":/src -w /src edgelet-embed-ci ./scripts/ci
```

---

## install.sh

Default: **`--flavor=full`**.

| Flag | Meaning |
|------|---------|
| `--flavor=full\|lite` | Must match tarball |
| `--arch=` | Override auto-detected arch |
| `--airgap` | Do not download; requires `--tarball-path` |
| `--tarball-path=PATH` | Local `.tar.gz` |
| `--upgrade` / `--rollback` | Same-flavor OTA |
| `--non-interactive` | Pot-oriented install |
| `--controller-url=` / `--provision-key=` | Optional convenience flags |

Install paths:

| Item | Path |
|------|------|
| Binary | `/usr/local/bin/edgelet` |
| Unit | `edgelet.service` → `ExecStart=/usr/local/bin/edgelet` |
| Config | `/etc/edgelet/config.yaml` |
| Data | `/var/lib/edgelet/` |
| Backups | `/var/backups/edgelet/` |

---

## Sample configs

| File | Use |
|------|-----|
| [config_lite.yaml](edgelet/etc/edgelet/config_lite.yaml) | Lite / docker or podman |
| [config_full.yaml](edgelet/etc/edgelet/config_full.yaml) | Full / edgelet engine |
| [config_new.yaml](edgelet/etc/edgelet/config_new.yaml) | Full multi-profile sample |

---

## systemd

Unit template: [packaging/systemd/edgelet.service](../systemd/edgelet.service)

Lite flavor adds `Wants=docker.service` or `Wants=podman.socket` as needed.

---

## CI

- **`scripts/ci`** — linux gate: embed pipeline → build-edgelet → `go test` → size check (≤55 MB amd64/arm64 full)
- **`.github/workflows/ci-go.yml`** — unit tests + optional Docker embed job
- **`make test-embedded-ci`** — Lima VM embedded containerd tests (macOS)
