# Edgelet integration testing

This document describes how to run Edgelet integration and end-to-end test suites.

## Test suites

| Suite | Location | Purpose |
|-------|----------|---------|
| Unit + package tests | `./...` via `make test-unit` | Fast regression gate |
| Docker integration | [`test/integration/`](../../test/integration/) | External engine connectivity and controller HTTP |
| Embedded engine IT | [`test/embedded/`](../../test/embedded/) | Lima VM pipeline for `containerEngine: edgelet` |
| Control plane IT | [`test/control-plane/`](../../test/control-plane/) | PoT controller reconciliation flows |
| Release smoke | [`test/release/`](../../test/release/) | Per-arch binary smoke after release build |

See each directory's README for prerequisites and commands.

## Docker integration tests

Requires a running Docker (or compatible) daemon and optional PoT controller for controller tests.

```bash
go test -v ./test/integration/...
```

Skip with the short flag:

```bash
go test -short ./...
```

## Embedded integration tests (macOS)

Full embedded-engine pipeline (Lima VM, embed build, install, assertions):

```bash
# From repository root:
./test/embedded/run-all.sh
```

Hybrid cgroup v1 coverage:

```bash
./test/embedded/run-all-cgroup-v1.sh
```

## Consolidated runners

```bash
./test/run-all.sh --suite=unit
./test/run-all.sh --suite=embedded
./test/run-all.sh --suite=control-plane
```

## Environment

Integration tests may require:

- Docker or Podman daemon access (desktop and external-engine paths)
- Network connectivity to a PoT controller (controller tests only)
- Edgelet configuration under `/etc/edgelet/` (embedded IT installs via `install.sh`)
- Lima on macOS for embedded suites

Configure your environment before running long-running suites.
