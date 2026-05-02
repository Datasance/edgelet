# ioFog Agent — Go Implementation

[![agent-go CI](https://github.com/datasance/agent/actions/workflows/agent-go.yml/badge.svg)](https://github.com/datasance/agent/actions/workflows/agent-go.yml)

Go implementation of the ioFog Agent, migrated from Java. The agent provides edge-computing capabilities for the ioFog platform — managing microservice containers, communicating with the Controller, and reporting system status.

## Project Structure

```
agent-go/
├── cmd/
│   ├── iofog-agent/         # CLI entry point  (iofog-agent status / info / …)
│   └── iofog-agentd/        # Daemon entry point (embeds containerd when iofog engine)
├── internal/                # Internal packages
│   ├── auth/                # TLS / JWT / certificate management
│   ├── buildmeta/           # Compile-time flavor (lite | full) for validation / version output
│   ├── config/              # Configuration loading and persistence
│   ├── constants/           # Shared constants (engine paths, network names)
│   ├── embedded/            # Embedded binary assets and extraction logic
│   ├── fieldagent/          # Controller communication and sync
│   ├── localapi/            # Local HTTP/WebSocket API (port 54321)
│   ├── models/              # Shared data models
│   ├── network/             # Network interface management
│   ├── processmanager/      # Container lifecycle and reconciliation
│   ├── proxy/               # SSH tunnel / proxy management
│   ├── resourceconsumption/ # CPU / memory / disk monitoring
│   ├── statusreporter/      # Status aggregation and reporting
│   ├── store/               # SQLite local storage
│   └── volumemount/         # Secret / ConfigMap volume mounts
├── pkg/
│   ├── containerd/          # In-process containerd service (iofog engine)
│   ├── docker/              # Docker client wrapper (legacy, kept for pkg/engine/docker)
│   └── engine/              # ContainerEngine interface + Docker/Podman/iofog adapters
│       ├── engine.go        # ContainerEngine interface definition
│       ├── factory.go       # NewContainerEngine() factory
│       ├── docker/          # Docker adapter (wraps pkg/docker)
│       ├── podman/          # Podman adapter (Docker-compatible socket)
│       └── iofog/           # Embedded containerd adapter
├── build/                   # Build scripts and Dockerfiles
│   ├── download-deps.sh     # Downloads embedded binaries for all 6 targets
│   └── containerd.Dockerfile # ARM32 cross-compile Dockerfile for containerd
├── install.sh               # One-line installer with multi-init-system support
├── uninstall.sh             # Clean uninstaller
├── packaging/               # Default config template
├── .golangci.yaml           # golangci-lint configuration
├── go.mod                   # Go module definition
├── Makefile                 # Build automation
├── Dockerfile               # Production image
└── Dockerfile.dev           # Development image (hot-reload)
```

## Prerequisites

| Tool        | Minimum version | Notes                              |
|-------------|-----------------|-------------------------------------|
| Go          | **1.24**        | Specified in `go.mod`              |
| Make        | any             | GNU Make                           |
| Docker / Podman | 26.10+     | Required for `docker`/`podman` engine modes |
| golangci-lint | v1.64.4       | Auto-installed by `make lint`      |
| gcc + musl cross-compilers | — | Required for `CGO_ENABLED=1` daemon builds |

For `iofog` (embedded) engine builds, install cross-compilers before building:

```bash
# Ubuntu / Debian
sudo apt-get install -y \
  gcc gcc-aarch64-linux-gnu gcc-arm-linux-gnueabihf gcc-riscv64-linux-gnu \
  musl-tools musl-dev
```

## Container Engine

There are two **build flavors** (set at compile time via `-ldflags`; see `internal/buildmeta`). The running config must match the binary:

| Build flavor | Allowed `containerEngine` | Notes |
|--------------|----------------------------|--------|
| **full** (default) | `iofog` only | CGO + embedded containerd; `dockerUrl` must be `unix:///run/iofog-agent/containerd.sock` |
| **lite** | `docker` or `podman` only | CGO disabled; connect to host Docker/Podman socket |

| Value | Description |
|-------|-------------|
| `docker` | Host Docker daemon (`dockerUrl` e.g. `unix:///var/run/docker.sock`) |
| `podman` | Host Podman (`dockerUrl` e.g. `unix:///run/podman/podman.sock`) |
| `iofog` | Embedded containerd + runc + CNI + Spin (WebAssembly) — **full flavor only** |

```yaml
profiles:
  production:
    containerEngine: iofog
    dockerUrl: unix:///run/iofog-agent/containerd.sock   # required when engine is iofog
```

### Embedded Engine (iofog)

When `containerEngine: iofog` is selected, `iofog-agentd` starts an in-process containerd instance. No external container runtime is required. The binary bundles:

- `containerd-shim-runc-v2` — OCI runtime shim
- `runc` — low-level OCI container runtime
- CNI plugins: `bridge`, `host-local`, `portmap`, `loopback`
- `containerd-shim-spin` — WebAssembly/WASI runtime (via [spinframework](https://github.com/spinframework/containerd-shim-spin))

**Path layout** (isolated from any host Docker/Podman installation):

| Path | Purpose |
|------|---------|
| `/var/lib/iofog-agent/` | User data (`diskDirectory`, subject to `diskConsumptionLimit`) |
| `/var/lib/iofog-agent-containerd/` | Containerd images and snapshots (not counted against disk limit) |
| `/run/iofog-agent/containerd.sock` | Ephemeral containerd socket |

The `iofog` engine uses a private bridge network (`iofog0`, CIDR `172.18.0.0/16`) and never conflicts with existing Docker or Podman installations. A warning is logged if Docker/Podman sockets are detected.

## Building

`FLAVOR` defaults to **`full`** (embedded containerd). Use **`lite`** for external Docker/Podman only.

```bash
make build                 # CLI + daemon for FLAVOR (default: full)
make build FLAVOR=lite     # lite daemon (CGO=0) + CLI with lite metadata

make build-cli             # CLI only (honors FLAVOR= for buildmeta)
make build-daemon          # Alias for build-daemon-$(FLAVOR)
make build-daemon-lite     # Daemon only, CGO=0
make build-daemon-full     # Daemon only, CGO=1 + embedded deps
```

### Embedded dependencies (full flavor)

```bash
make deps ARCH=amd64      # Download containerd shims, runc, CNI, etc.
make build-daemon-full    # Same as build-daemon-embedded (alias)
```

### Cross-compilation (all 6 arch targets, **lite + full** per target)

Each target produces four binaries, e.g. `build/iofog-agent-linux-amd64-lite`, `...-full`, `iofog-agentd-linux-amd64-lite`, `...-full`.

```bash
make build-all-archs      # all arches: lite + full each
make build-linux-amd64
make build-linux-amd64-musl
make build-linux-arm64
make build-linux-arm64-musl
make build-linux-arm
make build-linux-riscv64
```

### Release tarballs + checksum manifests

```bash
make release-tarballs VERSION=v3.8.0   # requires prior build-all-archs; outputs under build/release/
# SHA256SUMS-lite and SHA256SUMS-full
```

## Testing

```bash
make test             # Run all tests
make test-unit        # Run unit tests only (go test -short)
make test-coverage    # Run tests with HTML coverage report → build/coverage.html
make benchmark        # Run benchmarks
```

## Code Quality

```bash
make fmt              # Format code with gofmt
make vet              # Run go vet
make lint             # Run golangci-lint (auto-installs if absent)
make lint-fix         # Run golangci-lint with --fix
```

`make lint` downloads the pinned binary (`v1.64.4`) to `$GOBIN` via the official
install script the first time it is run — no manual installation required.

To override the pinned version:

```bash
make lint GOLANGCI_LINT_VERSION=v1.64.4
```

## Local Development

### Setup & Start

```bash
make build install-dev start-dev
```

This builds both binaries, installs them to `/usr/local/bin/`, creates the local
development directory tree under `dev/`, and starts the daemon.

```bash
export SNAP_COMMON=$(pwd)/dev
iofog-agent status
iofog-agent info
```

### Logs

```bash
tail -f dev/var/log/iofog-agent/daemon-startup.log
```

### Stop

```bash
make stop-dev
```

## Docker

```bash
make docker-build      # Build production image (iofog-agent-go:latest)
make docker-build-dev  # Build development image (iofog-agent-go:dev)
```

The production image is based on Alpine Linux, statically linked
(`CGO_ENABLED=0`), and targets < 30 MB.

## Installation

Distribution is **tarball + `install.sh` only** (no DEB/RPM in this product line). Default install flavor is **`full`**.

Tarball names:

`iofog-agent-<VERSION>-linux-<ARCH>[-musl]-<flavor>.tar.gz`  

where `<flavor>` is `lite` or `full`.

### Installer (from repo or release)

```bash
curl -fsSL https://raw.githubusercontent.com/datasance/agent/main/agent-go/install.sh | sudo sh -s -- --flavor=full
# or after copying install.sh locally:
sudo sh install.sh --flavor=full --version=v2.0.0
```

Common options:

| Flag | Purpose |
|------|---------|
| `--flavor=full\|lite` | Must match the tarball (default: **full**) |
| `--container-engine=docker\|podman` | For **lite** only; **full** always uses `iofog` |
| `--airgap` | Do not download; requires `--tarball-path` |
| `--tarball-path=PATH` | Local `.tar.gz` (offline) |
| `--upgrade` / `--rollback` | In-place upgrade / rollback using metadata below |
| `--expected-sha256` / `--checksum-path` | Optional verification for airgap tarballs |

Supported init systems: **`systemd`**, **OpenRC**, **SysV init**, **s6**, **runit**, **upstart**.  
On systemd, **Docker** pulls in `docker.service`; **Podman** uses **`podman.socket`**.

### Metadata (no JSON, no Python)

Under `/var/backups/iofog-agent/`:

- **`install-receipt`** — `installed_version`, `flavor`, `source_url` (`https://...` or `file:///...`)
- **`previous-release`** — written on `--upgrade`: previous version/flavor, download URL, config backup path

Plain **key=value** lines (POSIX `sh`). See [`packaging/PACKAGING-STRUCTURE.md`](packaging/PACKAGING-STRUCTURE.md).

Changing **lite ↔ full** on the same host is not supported; uninstall and reinstall the correct flavor.

### Manual install from tarball

```bash
curl -fsSL -O "https://github.com/datasance/agent/releases/download/vX.Y.Z/iofog-agent-vX.Y.Z-linux-amd64-full.tar.gz"
tar -xzf iofog-agent-*-full.tar.gz
sudo install -m 755 iofog-agent  /usr/local/bin/
sudo install -m 755 iofog-agentd /usr/local/bin/
# Prefer install.sh for config, service, and metadata.
```

### Uninstall

```bash
sudo sh uninstall.sh               # Remove binaries and service, keep data
sudo sh uninstall.sh --remove-data # Also remove all data directories
```

### Development install

```bash
make build install-dev start-dev   # full flavor; dev config uses iofog + embedded socket
```

## Version Information

```bash
iofog-agent version       # prints version, build flavor, allowed containerEngine set
iofog-agentd version
```

## CI / CD

| Workflow | Location | Purpose |
|----------|----------|---------|
| **agent-go** | [`.github/workflows/agent-go.yml`](../.github/workflows/agent-go.yml) (repo root) | On changes under `agent-go/`: build **lite + full** for linux `amd64`, `arm64`, `arm`, `riscv64`, run unit tests |

Release packaging: `make build-all-archs` then `make release-tarballs VERSION=…` → per-flavor tarballs under `build/release/` plus **`SHA256SUMS-lite`** and **`SHA256SUMS-full`**.

Build matrix (each arch emits **-lite** and **-full** binaries):

| Target | GOOS/GOARCH | Libc | Notes |
|--------|-------------|------|--------|
| `amd64` | linux/amd64 | glibc | |
| `amd64-musl` | linux/amd64 | musl (static daemon for full) | |
| `arm64` | linux/arm64 | glibc | |
| `arm64-musl` | linux/arm64 | musl | |
| `arm` | linux/arm (armhf) | glibc | |
| `riscv64` | linux/riscv64 | glibc | spin shim excluded where unsupported |

## Migration Status

✅ **Migration Complete** — all functionality from the Java daemon has been ported to Go.

| Component | Status |
|---|---|
| Controller communication (Field Agent) | ✅ |
| Process Manager (container reconciliation) | ✅ |
| Local API (port 54321) | ✅ |
| SQLite local storage | ✅ |
| Volume mounts (Secrets / ConfigMaps) | ✅ |
| Resource consumption monitoring | ✅ |
| Network interface management | ✅ |
| SSH proxy / tunnel | ✅ |
| JWT authentication | ✅ |
| TLS certificate management | ✅ |
| Diagnostics & strace | ✅ |
| Security hardening (gosec) | ✅ |
| Tarball + `install.sh` (this repo) | ✅ |
| Legacy DEB / RPM (Java line) | separate |

## Performance vs Java

| Metric | Go (docker/podman) | Go (iofog) | Java |
|---|---|---|---|
| Binary size | < 30 MB | ~80–120 MB (with embedded runtimes) | ~200 MB (+ JRE) |
| Memory at idle | < 100 MB | ~150 MB (containerd overhead) | ~300 MB |
| Startup time | < 2 s | ~4 s (containerd init) | ~5 s |
| CPU at idle | < 1 % | < 1 % | ~2–3 % |

The iofog binary is larger because it bundles containerd shims, runc, and CNI plugins. This is expected and intentional — the embedded engine requires no pre-installed container runtime on the host.

## Security

Static analysis runs on every CI build via `gosec` (integrated into `golangci-lint`).
All `#nosec` suppressions carry a justification comment explaining why the rule
is intentionally bypassed.

## Contributing

See `CONTRIBUTING.md` for contribution guidelines.

## License

See `LICENSE` in the repository root.
