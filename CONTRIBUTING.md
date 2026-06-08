# Contributing to Edgelet

Thank you for contributing to **Edgelet** (`github.com/eclipse-iofog/edgelet`) — the greenfield edge runtime and PoT node agent. This repository is **not** a Java parity port; all source lives at the repository root.

## Branch workflow

1. Fork [github.com/eclipse-iofog/edgelet](https://github.com/eclipse-iofog/edgelet).
2. Clone your fork and create a feature branch from **`develop`**.
3. Make focused changes with tests where behavior is non-obvious.
4. Run the gates below before opening a pull request.
5. Open a PR against **`develop`** with a clear description and test notes.

## Required gates

Run these from the repository root:

```bash
make fmt
make lint              # golangci-lint — zero issues required
make security-code     # gosec on ./cmd ./internal ./pkg
make vulncheck         # govulncheck + go mod verify
make test-unit
make cli-docs-check    # if CLI commands or help text changed
make cli-help-check
```

Full CI parity on macOS (Linux embed tests inside Docker):

```bash
make ci-docker
```

Embedded integration tests (Lima VM): [test/embedded/README.md](test/embedded/README.md).

## macOS and the embed pipeline

Linux thin binaries embed a zstd fat runtime. On **macOS**, `make build-all-archs` fails without native Linux cross-toolchains. Use the Docker-based release path instead:

```bash
./test/release/build-all.sh
# or: make build-release-matrix
```

After changing `scripts/install-embed-build-deps`, rebuild CI images:

```bash
RELEASE_FRESH_CI_IMAGE=1 ./test/release/build-all.sh
```

Desktop development on macOS uses a monolithic build with Docker Desktop, Podman Machine, or OrbStack:

```bash
make install-dev start-dev
```

## Code style

- Follow standard Go formatting (`go fmt` / `make fmt`).
- Run `go vet` and `make lint` before submitting.
- Use clear names; comment exported APIs and non-obvious logic.
- Keep functions focused; match existing package layout under `cmd/`, `internal/`, `pkg/`.

## Testing

- Add or extend tests for new behavior.
- Prefer table-driven unit tests in the same package or `_test` package as surrounding code.
- Do not rely on Controller or cloud services for unit tests.

## Pull requests

1. Update **CHANGELOG.md** under `[Unreleased]` only if your change is user-facing and lands before the next tagged release; release prep may fold entries into the beta section.
2. Update operator docs under `docs/edgelet/` or `docs/cli/` when CLI or install behavior changes.
3. Regenerate CLI docs when Cobra commands change: `make cli-docs`.
4. Ensure all required gates pass locally or in CI.

## Security

Report vulnerabilities per [SECURITY.md](SECURITY.md) — do not file public issues for exploitable findings.

## Questions

Open a GitHub issue for bugs and feature discussion, or contact the maintainers listed in the repository.
