# ioFog Agent — Go Implementation

[![CI](https://github.com/eclipse-iofog/agent/actions/workflows/ci-go.yml/badge.svg)](https://github.com/eclipse-iofog/agent/actions/workflows/ci-go.yml)
[![Build](https://github.com/eclipse-iofog/agent/actions/workflows/build.yml/badge.svg)](https://github.com/eclipse-iofog/agent/actions/workflows/build.yml)

Go implementation of the ioFog Agent, migrated from Java. The agent provides edge-computing capabilities for the ioFog platform — managing microservice containers, communicating with the Controller, and reporting system status.

## Project Structure

```
agent-go/
├── cmd/
│   ├── iofog-agent/         # CLI entry point  (iofog-agent status / info / …)
│   └── iofog-agentd/        # Daemon entry point
├── internal/                # Internal packages
│   ├── auth/                # TLS / JWT / certificate management
│   ├── config/              # Configuration loading and persistence
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
│   └── docker/              # Docker client wrapper
├── build/                   # Build artifacts (git-ignored)
├── packaging/               # DEB / RPM packaging scripts
├── .golangci.yaml           # golangci-lint configuration
├── go.mod                   # Go module definition
├── Makefile                 # Build automation
├── Dockerfile               # Production image (Alpine, CGO_ENABLED=0)
└── Dockerfile.dev           # Development image (hot-reload)
```

## Prerequisites

| Tool        | Minimum version | Notes                            |
|-------------|-----------------|----------------------------------|
| Go          | **1.24**        | Specified in `go.mod`            |
| Make        | any             | GNU Make                         |
| Docker      | 26.10+          | Required for container management|
| golangci-lint | v1.64.4       | Auto-installed by `make lint`    |

## Building

```bash
make build           # Build both binaries → build/iofog-agent, build/iofog-agentd
make build-cli       # Build CLI binary only
make build-daemon    # Build daemon binary only
```

### Cross-Compilation

```bash
GOOS=linux GOARCH=arm64  CGO_ENABLED=0 go build ./cmd/iofog-agent
GOOS=linux GOARCH=arm    GOARM=7 CGO_ENABLED=0 go build ./cmd/iofog-agentd
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

```bash
make install           # sudo-copies both binaries to /usr/local/bin/
```

## Version Information

```bash
iofog-agent version
iofog-agentd version
```

## CI / CD

| Workflow | Trigger | Jobs |
|---|---|---|
| **CI** (`ci-go.yml`) | push / PR → `main`, `develop` | lint → test → build → docker |
| **Build** (`build.yml`) | push tag `v*` / manual | lint → build (multi-arch) → docker → packages |
| **Release** (`release.yml`) | push tag `v*` | attach artifacts to GitHub Release |

Every CI run:
1. Verifies the Go version (`go version`)
2. Runs `golangci-lint` v1.64.4 via the official `golangci/golangci-lint-action@v3`
3. Runs the full test suite
4. Cross-compiles for `linux/amd64`, `linux/arm64`, `linux/arm/v7`

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
| DEB / RPM packaging | ✅ |

## Performance vs Java

| Metric | Go | Java |
|---|---|---|
| Binary size | < 50 MB | ~200 MB (+ JRE) |
| Memory at idle | < 100 MB | ~300 MB |
| Startup time | < 2 s | ~5 s |
| CPU at idle | < 1 % | ~2–3 % |

## Security

Static analysis runs on every CI build via `gosec` (integrated into `golangci-lint`).
All `#nosec` suppressions carry a justification comment explaining why the rule
is intentionally bypassed.

## Contributing

See `CONTRIBUTING.md` for contribution guidelines.

## License

See `LICENSE` in the repository root.
