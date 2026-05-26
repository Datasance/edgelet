# Edgelet

[![agent-go CI](https://github.com/datasance/agent/actions/workflows/agent-go.yml/badge.svg)](https://github.com/datasance/agent/actions/workflows/agent-go.yml)

Greenfield edge agent for the ioFog platform — single multicall `edgelet` binary (CLI + daemon + embedded containerd child on full/linux). Manages microservice containers, syncs with the Controller, and exposes the on-device **EdgeletAPI** for local administration.

**Documentation:** [docs/edgelet/README.md](docs/edgelet/README.md)

## Project structure

```
.
├── cmd/edgelet/             # Multicall entry (CLI, daemon, containerd child)
├── internal/
│   ├── auth/                # TLS / JWT / EdgeletAPI PKI
│   ├── buildmeta/           # Compile-time flavor (lite | full)
│   ├── edgeletapi/          # EdgeletAPI HTTP/WebSocket (:54321)
│   ├── fieldagent/          # Controller communication
│   ├── processmanager/      # Container reconciliation
│   ├── store/               # SQLite persistence
│   └── supervisor/          # Module orchestration
├── pkg/
│   ├── containerd/          # In-process containerd (edgelet engine)
│   └── engine/              # ContainerEngine + docker/podman/edgelet adapters
├── install.sh               # Tarball installer
├── uninstall.sh             # Clean uninstaller
├── packaging/               # systemd unit, config templates
└── docs/edgelet/            # Operator documentation
```

## Prerequisites

| Tool | Minimum version | Notes |
|------|-----------------|-------|
| Go | **1.24** | In `go.mod` |
| Make | any | GNU Make |
| Docker / Podman | 26.10+ | Lite flavor only |
| golangci-lint | v1.64.4 | Auto-installed by `make lint` |

For **full** (embedded) engine builds on Linux, install cross-compilers before building:

```bash
sudo apt-get install -y \
  gcc gcc-aarch64-linux-gnu gcc-arm-linux-gnueabihf gcc-riscv64-linux-gnu \
  musl-tools musl-dev
```

## Container engine

Two **build flavors** (compile-time via `internal/buildmeta`). Config must match the binary:

| Build flavor | Allowed `containerEngine` | Notes |
|--------------|----------------------------|--------|
| **full** (default, linux) | `edgelet` only | CGO + embedded containerd |
| **lite** | `docker` or `podman` | CGO disabled; external engine |

| Value | Description |
|-------|-------------|
| `docker` | Host Docker (`dockerUrl` e.g. `unix:///var/run/docker.sock`) |
| `podman` | Host Podman |
| `edgelet` | Embedded containerd — **full flavor only** |

```yaml
profiles:
  production:
    containerEngine: edgelet
    dockerUrl: unix:///run/edgelet/containerd.sock
```

Details: [docs/edgelet/container-engine.md](docs/edgelet/container-engine.md)

## Building

```bash
make build                 # CLI + daemon for FLAVOR (default: full)
make build FLAVOR=lite     # lite daemon + CLI

make build-cli
make build-daemon-full     # CGO=1 + embedded deps
make build-daemon-lite     # CGO=0
```

Cross-compilation (lite + full per arch):

```bash
make build-all-archs
make release-tarballs VERSION=v1.0.0
```

## Testing

```bash
make test
make test-unit
make test-coverage
```

Embedded full-flavor integration (Lima VM on macOS): [test/embedded/README.md](test/embedded/README.md)

## Local development

Non-Linux hosts use **lite** flavor (`containerEngine: docker`):

```bash
make install-dev start-dev
export SNAP_COMMON=$(pwd)/dev
edgelet system status
edgelet system info
```

CLI reference: [docs/cli/README.md](docs/cli/README.md) · [output schemas](docs/cli/output-schemas.md) · [CLI migration](docs/edgelet/migration-from-iofog-agent-cli.md)

```bash
make stop-dev
tail -f dev/var/log/edgelet/daemon-startup.log
```

## Installation

Tarball + `install.sh` only (no DEB/RPM). Default flavor: **full**.

```bash
curl -fsSL https://raw.githubusercontent.com/datasance/agent/main/install.sh | sudo sh -s -- --flavor=full
sudo sh install.sh --flavor=full --tarball-path=edgelet-linux-amd64-full.tar.gz
```

Tarball names: `edgelet-<VERSION>-linux-<ARCH>-{full|lite}.tar.gz`

```bash
edgelet daemon                    # foreground
systemctl start edgelet           # production
edgelet provision                 # register with Controller
```

Deployment guide: [docs/edgelet/deployment.md](docs/edgelet/deployment.md)

### Uninstall

```bash
sudo sh uninstall.sh
sudo sh uninstall.sh --remove-data
```

## EdgeletAPI

On-device operator API (daemon↔CLI):

| Item | Value |
|------|--------|
| HTTPS | `https://127.0.0.1:54321` |
| Routes | `/v1/...` |
| CLI token | `/etc/edgelet/edgelet-api` |
| TLS CA | `/etc/edgelet/edgeletapi-ca.crt` |

Guide: [docs/edgelet/edgelet-api-v1.md](docs/edgelet/edgelet-api-v1.md) · OpenAPI: [docs/edgelet/edgelet-api-v1-openapi.yaml](docs/edgelet/edgelet-api-v1-openapi.yaml)

Controller REST (field agent) remains `/api/v3/...` on the controller URL — separate from EdgeletAPI.

## Code quality

```bash
make fmt
make vet
make lint
```

## CI

| Workflow | Purpose |
|----------|---------|
| `.github/workflows/ci-go.yml` | Build lite + full, unit tests |
| `.github/workflows/build.yml` | Release matrix |

## License

See `LICENSE` in the repository root.
