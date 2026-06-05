# Edgelet

[![agent-go CI](https://github.com/datasance/agent/actions/workflows/agent-go.yml/badge.svg)](https://github.com/datasance/agent/actions/workflows/agent-go.yml)

Greenfield edge agent for the ioFog platform — single `edgelet` binary per platform (linux: thin CLI + embed + fat runtime; desktop: monolithic). Manages microservice containers, syncs with the Controller, and exposes the on-device **EdgeletAPI** for local administration.

**Documentation:** [docs/edgelet/README.md](docs/edgelet/README.md)

## Project structure

```
.
├── cmd/edgelet/             # Multicall entry (CLI, daemon, containerd child)
├── internal/
│   ├── auth/                # TLS / JWT / EdgeletAPI PKI
│   ├── buildmeta/           # Platform capability (embedded engine, allowed engines)
│   ├── edgeletapi/          # EdgeletAPI HTTP/WebSocket (:54321)
│   ├── fieldagent/          # Controller communication
│   ├── processmanager/      # Container reconciliation
│   ├── store/               # SQLite persistence
│   └── supervisor/          # Module orchestration
├── pkg/
│   ├── containerd/          # In-process containerd (edgelet engine)
│   └── engine/              # ContainerEngine + docker/podman/edgelet adapters
├── install.sh               # Binary installer (multi-OS)
├── uninstall.sh             # Clean uninstaller
├── packaging/               # systemd unit, config templates
└── docs/edgelet/            # Operator documentation
```

## Prerequisites

| Tool | Minimum version | Notes |
|------|-----------------|-------|
| Go | **1.24** | In `go.mod` |
| Make | any | GNU Make |
| Docker / Podman | 26.10+ | When `containerEngine` is docker/podman |
| golangci-lint | v1.64.4 | Auto-installed by `make lint` |

For **linux** thin builds with embedded runtime, install cross-compilers before building:

```bash
sudo apt-get install -y \
  gcc gcc-aarch64-linux-gnu gcc-arm-linux-gnueabihf gcc-riscv64-linux-gnu \
  musl-tools musl-dev
```

## Container engine

Runtime selection via `containerEngine` in config (validated per GOOS):

| Platform | Allowed `containerEngine` | Default |
|----------|---------------------------|---------|
| **linux** | `edgelet`, `docker`, `podman` | **`edgelet`** |
| **darwin / windows** | `docker`, `podman` | `docker` |

| Value | Description |
|-------|-------------|
| `edgelet` | Embedded containerd (linux only) |
| `docker` | Host Docker (`containerEngineUrl` e.g. `unix:///var/run/docker.sock`) |
| `podman` | Host Podman (`containerEngineUrl` e.g. `unix:///run/podman/podman.sock`) |

```yaml
profiles:
  production:
    containerEngine: edgelet
    containerEngineUrl: unix:///run/edgelet/containerd.sock
    pruningFrequency: 24
    watchdogEnabled: true
```

Details: [docs/edgelet/container-engine.md](docs/edgelet/container-engine.md)

## Building

```bash
make build                    # host OS: linux thin or desktop monolithic
make build-edgelet-linux      # unified linux thin (ARCH=amd64 default)
make deps                     # embed pipeline before linux thin build

make build-cli                # alias for local edgelet
make build-daemon-embedded    # alias for build-edgelet-linux
make build-linux-amd64        # deps + thin for amd64
make build-linux-arm64        # deps + thin for arm64
make build-all-archs          # linux matrix (amd64, arm64, arm, riscv64)
make build-desktop-darwin     # darwin monolithic
make release-binaries VERSION=v1.0.0
```

## Testing

```bash
make test
make test-unit
make test-coverage
```

Embedded-engine integration (Lima VM on macOS): [test/embedded/README.md](test/embedded/README.md)

## Local development

Non-Linux hosts use desktop monolithic build (`containerEngine: docker`):

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

Binary-only releases on **`datasance/edgelet`** (no DEB/RPM, no `.tar.gz` bundles). Default linux engine: **edgelet**.

```bash
curl -fsSL https://github.com/datasance/edgelet/releases/download/vX.Y.Z/install.sh -o install.sh
chmod +x install.sh
sudo ./install.sh --version=vX.Y.Z
# dev / CI: sudo ./install.sh --bin-path=build/edgelet-linux-amd64 --version=dev
```

Release binaries: `edgelet-linux-<arch>`, `edgelet-darwin-<arch>`, `edgelet-windows-amd64.exe` + `SHA256SUMS` + config/CA samples.

```bash
sudo edgelet init-config          # default config if missing
edgelet daemon                    # foreground
systemctl start edgelet           # production (linux)
edgelet provision <key>           # register with Controller (post-install)
```

Deployment guide: [docs/edgelet/deployment.md](docs/edgelet/deployment.md)

Container image (linux IT / nested deploy): `ghcr.io/datasance/edgelet:<tag>`, built from root `Dockerfile`. Local tag: `edgelet:local`.

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
| `.github/workflows/ci-go.yml` | Build linux + desktop, unit tests |
| `.github/workflows/build.yml` | Release matrix |

## License

See `LICENSE` in the repository root.
