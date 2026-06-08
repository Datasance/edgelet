# Changelog

All notable changes to Edgelet are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0-beta.0] — mid-June 2026

First public pre-release of **Edgelet** — a greenfield edge runtime and ioFog/PoT node agent (`github.com/datasance/edgelet`). GitHub Releases are marked **Pre-release** and ship binary-only artifacts (no DEB/RPM, no release tarballs).

### Added

- **Single `edgelet` binary** per platform: Linux thin wrapper (~31 MiB download) with k3s-style zstd embed and lazy extract to `/var/lib/edgelet/data/current/`; macOS and Windows monolithic builds for external Docker/Podman only.
- **Embedded Linux runtime** (`containerEngine: edgelet`, default on Linux): in-process containerd (k3s fork), CRI socket at `/run/edgelet/containerd.sock`, static `crun` 1.28 in the embed bundle.
- **Multi-engine support on Linux:** `edgelet`, `docker`, or `podman` via config; desktop platforms support `docker` and `podman` only.
- **EdgeletAPI** on-device operator API at `https://127.0.0.1:54321` with routes under `/v1/...`, RBAC group `edgelet.iofog.org/v1`, TLS/PKI under `/etc/edgelet/`.
- **Field agent** for Controller sync; Pot REST remains at `/api/v3/...` on the controller URL (unchanged contract).
- **Operator CLI** (Cobra): grouped commands (`system`, `ms`, `image`, `registry`, `runtimeclass`, `config`, `deploy`, `provision`), structured `-o json|yaml`, shell completion, generated docs under `docs/cli/`.
- **`edgelet init-config`:** write default config from template when missing (idempotent).
- **Binary-only install/OTA:** `install.sh` / `uninstall.sh` for Linux, macOS, and Windows; `--upgrade` / `--rollback` with receipt under `/var/backups/edgelet/`; six Linux init system templates.
- **Release packaging:** `scripts/release-binaries.sh` produces seven binaries + `SHA256SUMS` + config/CA samples.
- **Seven release binaries:** `edgelet-linux-{amd64,arm64,arm,riscv64}`, `edgelet-darwin-{amd64,arm64}`, `edgelet-windows-amd64.exe`.
- **Container image:** `ghcr.io/datasance/edgelet-linux:<tag>` (scratch base, `EDGELET_DAEMON=container`).
- **macOS release build path:** `test/release/build-all.sh` (Docker embed loop) for developers without native Linux cross-toolchains.
- **Arch smoke scripts:** `test/release/smoke-linux-{arm,riscv64}.sh` for post-build daemon, version, and CRI socket checks.
- **Security gates:** `make security-code` (gosec), `make vulncheck` (govulncheck), CI workflow for vulnerability scanning; see [SECURITY.md](SECURITY.md).
- **SQLite persistence** for local state, deploy manifests, and runtime metadata.
- **Local deploy manifest** uses single `spec.image` (no per-arch image maps).
- **FogType / arch mapping:** amd64, arm64, riscv64, arm (32-bit) for provision and status display.

### Changed

- **Product identity:** greenfield rebrand to Edgelet — paths under `/var/lib/edgelet`, `/etc/edgelet`, `/run/edgelet`; labels and env vars use `edgelet.iofog.org/*` and `EDGELET_*`.
- **Daemon entry:** bare `edgelet` invokes the CLI; start the daemon with `edgelet daemon` or `systemctl start edgelet`.
- **Provision payload** includes configured `containerEngine` and build metadata sent to the Controller.
- **CLI redesign (breaking vs legacy ioFog Agent CLI):** command groups and flags replaced flat `iofog-agent` verbs; see [docs/edgelet/migration-from-iofog-agent-cli.md](docs/edgelet/migration-from-iofog-agent-cli.md). Daemon unreachable returns exit code **10**.
- **Documentation:** operator docs under `docs/edgelet/` (architecture, deployment, EdgeletAPI, container engine).
- **Toolchain:** Go 1.26.x; containerd **v2.2.3-k3s1** with pinned CRI API replacements.
- **Quality tooling:** golangci-lint v2 (govet, revive, staticcheck, errcheck, formatters, misspell, errorlint); gosec run separately from lint.

### Fixed

- **Embed packaging:** single `.tar.zst` per arch in `go:embed` (prevents multi-arch artifact accumulation inflating binary size).
- **CRI lifecycle:** microservice restart and stop/start on the embedded engine use remove+create+start instead of failing with non-restartable container errors.
- **Logging on arm32:** size cap avoids int overflow on 32-bit platforms.
- **Embed cross-build on macOS:** host-arch tooling and zlib dev packages for arm/riscv64 fat-runtime link.
- **gosec and vulnerability findings** addressed across `cmd/`, `internal/`, and `pkg/`; documented exceptions only where noted in [SECURITY.md](SECURITY.md).

### Known limitations (beta)

- **Pre-release:** `v1.0.0-beta.0` is not a production GA; expect API and packaging refinements before 1.0.0.
- **Windows (Tier 2):** `edgelet-windows-amd64.exe` is built and published; there is no Windows integration-test matrix or Windows service installer in this release.
- **macOS:** supported as a **development platform** with external Docker/Podman only — not positioned as a production far-edge node OS.
- **linux/arm smoke depth:** arm32 (`edgelet-linux-arm`) builds in the release matrix; full arch smoke is validated on Linux with binfmt or native hardware. Running arm smoke under macOS Docker/QEMU may segfault after embed extract even when the binary build succeeds.
- **linux/riscv64:** release build and smoke script pass on macOS Docker; fleet validation on real riscv64 hardware is limited in beta.
- **Binary-only distribution:** no DEB/RPM packages and no release `.tar.gz` bundles; use `install.sh` or copy the raw binary.
- **OTA depth:** one previous release for thin-binary rollback; fat embed bundle keeps `current` / `previous` symlinks only.
- **Codecov:** coverage upload not wired; badge is a placeholder until post-beta CI work.
- **Dependency exceptions (docker/podman engine):** govulncheck documents two accepted findings in the pinned `github.com/docker/docker` client SDK (Moby AuthZ plugin advisories **GO-2026-4887** / **GO-2026-4883**, CVE-2026-34040). Edgelet uses the SDK as a **client** to local engines; typical edge deployments do not enable AuthZ plugins. Operators should run a patched Docker Engine (≥ 29.3.1) or equivalent Podman. Full rationale and fix timeline: [SECURITY.md](SECURITY.md).

### Binary size (linux thin download gate)

| Arch          | Thin binary | ≤ 55 MiB |
|---------------|-------------|----------|
| linux/amd64   | ~34.7 MiB   | yes      |
| linux/arm64   | ~31.7 MiB   | yes      |
| linux/riscv64 | ~32.2 MiB   | yes      |
| linux/arm     | ~31.8 MiB   | yes      |