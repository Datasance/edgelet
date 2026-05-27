# Packaging Structure — Edgelet

## Overview

Edgelet uses **tarball-only** distribution. **Linux** ships one unified artifact per architecture; **darwin/windows** ship a monolithic desktop binary.

| Platform | Artifact | `containerEngine` | Notes |
|----------|----------|-------------------|--------|
| **linux** | `edgelet-linux-<arch>.tar.gz` | `edgelet` (default), `docker`, `podman` | Thin OTA binary + embedded zstd fat runtime |
| **darwin** | `edgelet-darwin-<arch>.tar.gz` | `docker` or `podman` | Monolithic; no embed |
| **windows** | `edgelet-windows-amd64.tar.gz` | `docker` or `podman` | Monolithic; no embed |

No **musl-suffixed** release line (RFC R9). Optional ops-only static linking: `STATIC_BUILD=true make build-linux-amd64`.

DEB/RPM are not used. Installation is via **`install.sh`**; removal via **`uninstall.sh`**.

---

## Distribution artifacts

Release tarball names:

- **Linux:** `edgelet-linux-<arch>.tar.gz`
- **Darwin:** `edgelet-darwin-<arch>.tar.gz`
- **Windows:** `edgelet-windows-amd64.tar.gz`

Versioned copies: `edgelet-<VERSION>-linux-<arch>.tar.gz` (and desktop equivalents).

Examples:

- `edgelet-linux-amd64.tar.gz`
- `edgelet-linux-arm64.tar.gz`
- `edgelet-darwin-arm64.tar.gz`

Each linux tarball contains:

- `edgelet` — thin wrapper (CLI + embed + `daemon` dispatch)
- Optional `config.yaml.sample`

Checksum manifest (`make release-tarballs`):

- `dist/SHA256SUMS`

---

## Build

```bash
# Local default (host OS)
make build-edgelet-linux    # linux thin (needs deps embed on linux or Docker)
make build-edgelet-local    # host: linux thin or desktop monolithic

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

Default linux install uses **`edgelet-linux-<arch>.tar.gz`** (no flavor suffix).

| Flag | Meaning |
|------|---------|
| `--container-engine=` | `edgelet`, `docker`, or `podman` (linux default: **edgelet**) |
| `--flavor=full\|lite` | **Deprecated** — ignored |
| `--arch=` | Override auto-detected arch |
| `--airgap` | Do not download; requires `--tarball-path` |
| `--tarball-path=PATH` | Local `.tar.gz` |
| `--upgrade` / `--rollback` | OTA |
| `--non-interactive` | Pot-oriented install |
| `--controller-url=` / `--provision-key=` | Optional convenience flags |

Install paths:

| Item | Path |
|------|------|
| Binary | `/usr/local/bin/edgelet` |
| Unit | `edgelet.service` → `ExecStart=/usr/local/bin/edgelet daemon` |
| Config | `/etc/edgelet/config.yaml` |
| Data | `/var/lib/edgelet/` |
| Backups | `/var/backups/edgelet/` |

---

## Sample configs

| File | Use |
|------|-----|
| [config_lite.yaml](edgelet/etc/edgelet/config_lite.yaml) | External engine sample (docker/podman) |
| [config_full.yaml](edgelet/etc/edgelet/config_full.yaml) | Embedded engine sample (`edgelet`) |
| [config_new.yaml](edgelet/etc/edgelet/config_new.yaml) | Multi-profile sample |

---

## systemd

Unit template: [packaging/systemd/edgelet.service](../systemd/edgelet.service)

When `containerEngine` is docker or podman, use `After=docker.service` or `After=podman.service` in a drop-in (see [docs/edgelet/deployment.md](../docs/edgelet/deployment.md)).

---

## CI

- **`scripts/ci`** — linux gate: embed pipeline → build-edgelet → `go test` → size check (≤55 MB amd64/arm64 thin)
- **`.github/workflows/ci-go.yml`** — unit tests + optional Docker embed job
- **`make test-embedded-ci`** — Lima VM embedded containerd tests (macOS)
