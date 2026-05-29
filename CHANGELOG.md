# Changelog

All notable changes to the ioFog Agent Go implementation will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Embed crun 1.28:** Pin `VERSION_CRUN=1.28` (json-c ≥ 0.14). Build static glibc `crun` from source via `scripts/build-crun-static` for all linux embed arches (including arm32); removes curl prebuilts and dynamic native autotools output that broke scratch/Docker exec.
- **Plan 8 — release & install (binary-only):** GitHub Releases publish raw `edgelet-<os>-<arch>[.exe]` + `SHA256SUMS` + samples + `install.sh`/`uninstall.sh` (no release tarballs). Multi-OS `install.sh` with `--bin-path`, `--airgap`, `--upgrade`/`--rollback`; linux six init systems; OTA via `internal/version` `ReleaseManager` (no Java VC / apt). Local deploy manifest **`spec.image`** only. FogType 1–4 = amd64, arm64, riscv64, arm. Scratch `ghcr.io/datasance/edgelet-linux` container image.
- **Plan 7 — unified edgelet (drop full/lite):** Linux ships one binary per arch (`build/edgelet-linux-<arch>`) with runtime `containerEngine` selection (`edgelet`, `docker`, `podman`; default **`edgelet`**). Release tarballs renamed to **`edgelet-linux-<arch>.tar.gz`** (no `-full`/`-lite` suffix). `install.sh --flavor=` deprecated. Provision body sends `flavor: "edgelet"` with `engine` = configured value. Darwin/windows remain monolithic (docker/podman only).
- **Plan 6 — full linux two-layer binary (k3s-style):** Download artifact is a **thin** `edgelet` (`CGO=0`, CLI + `go:embed` zstd bundle + `daemon` dispatch). Runtime (**fat**) lives in the tar as `bin/edgelet` and extracts to `/var/lib/edgelet/data/<hash>/` with `current` / `previous` symlinks. Systemd remains `ExecStart=/usr/local/bin/edgelet daemon`. Operator CLI does not require extract; `--edgelet-containerd-child` runs from the fat ELF only.
- **Plan 6 — monolithic diet (full):** Docker/Podman engine packages build-tagged `lite` only; full factory accepts `containerEngine=edgelet` at compile time.
- EdgeletAPI code rename: Go packages, auth/PKI paths, CLI client symbols, and CI gates aligned with `.cursor/edgelet/NAMING.md` (HTTP routes remain `/v1/...`).
- Bare **`edgelet`** invokes the operator CLI (help banner); start the daemon explicitly with **`edgelet daemon`** or **`systemctl start edgelet`** (`ExecStart=…/edgelet daemon` in systemd/packaging).
- Documentation migration: obsolete root docs replaced by `docs/edgelet/` tree (architecture, deployment, EdgeletAPI v1, CLI migration); root `README.md` and hand-written CLI docs updated for Edgelet terminology.

### Added

- **`edgelet init-config`:** Write default config from embedded/sample template when `/etc/edgelet/config.yaml` is missing (idempotent).
- **`test/install/`:** `install-fresh-linux.sh`, `install-upgrade-rollback.sh`, `install-airgap.sh` for install/OTA script smoke.
- **`scripts/release-binaries.sh`:** Binary-only `dist/` packaging (replaces `release-tarballs.sh`).
- **`cmd/edgelet-server`:** Fat full-linux entry (supervisor, field agent, EdgeletAPI server, in-process containerd).
- **`scripts/check-containerd-fork.sh`:** CI guard that `go list -m` resolves containerd to `github.com/k3s-io/containerd/v2 v2.2.3-k3s1`.

## [1.0.0-edgelet] — TBD (proposed)

> **Plan 5 status (2026-05-27):** Binary size gate **PASS** after Plan 6 (thin full ≤ 55 MiB amd64/arm64). Pot staging sign-off and some install/OTA smokes still pending. See `.cursor/edgelet/docs/05-verification.md`.

### Fixed — embed packaging (Plan 5 P0)

- **`scripts/package-data`:** Clear prior `pkg/data/embed/*.tar.zst` before installing the new bundle (prevents `go:embed embed/*` from accumulating multi-arch artifacts ~2× ELF size).
- **`scripts/ci`:** Scope `EDGELET_CI_ARCHES="${_arch}"` per loop iteration; fail fast if embed dir does not contain exactly one `.tar.zst` before `build-edgelet`.

### Known blockers (pre-release)

- Staging Pot checklist (provision, volumeMounts/NATS, router+NATS MS) not yet signed off.
- Air-gapped `install.sh` smoke and some cross-arch daemon smokes pending linux VM (embedded IT green on primary arch).

### Binary size (Plan 7 — unified linux thin gate)

| Arch | Thin `edgelet-linux-*` (bytes) | ≤ 55 MiB |
|------|-------------------------------|----------|
| linux/amd64 | 32,690,338 | **yes** (~31.2 MiB) |
| linux/arm64 | 29,753,506 | **yes** (~28.4 MiB) |

Fat runtime ELF in tar (uncompressed, informational): amd64 **~50.5 MiB**, arm64 **~48.1 MiB** (Plan 6 reference). Desktop darwin/arm64: size gate n/a.

### Breaking (Plan 7)

- Linux release tarball names: `edgelet-linux-<arch>.tar.gz` replaces `edgelet-*-linux-<arch>-{full|lite}.tar.gz`.
- Compile-time full/lite flavor removed; use `containerEngine` in config instead.
- `install.sh --flavor=` ignored (deprecated).

### Binary size (Plan 6 — thin full download gate, superseded paths)

| Arch | Thin full (bytes) | ≤ 55 MiB |
|------|-------------------|----------|
| linux/amd64 | 32,448,674 | **yes** (~31.0 MiB) |
| linux/arm64 | 29,491,362 | **yes** (~28.1 MiB) |

Fat runtime ELF in tar (uncompressed): amd64 **50,489,304 B**, arm64 **47,230,392 B**.

### Added — Edgelet greenfield release (Plans 1–4)

- **`edgelet`** multicall: thin wrapper + extracted fat runtime on full linux; monolithic on lite; `--edgelet-containerd-child` on fat full linux.
- k3s-style zstd embed pipeline; release tarballs `edgelet-*-{full|lite}.tar.gz`.
- Greenfield **`install.sh`** + **`edgelet.service`**; paths under `/var/lib/edgelet/`.
- **`scripts/ci`** linux gate via Docker (`make ci-docker` on macOS).
- Local API **`/v1/`**, RBAC **`edgelet.iofog.org/v1`**, deploy **`edgelet.iofog.org/v1`** manifests.
- Pot field agent REST unchanged at **`/api/v3/…`**; full provision sends **`engine=edgelet`**.

### Fixed — Plan 5 verification (uncommitted)

- Makefile: lite builds inject **`FLAVOR=lite`** at link time (`build-edgelet-lite`, desktop lite targets).
- `internal/config/config_test.go`: flavor-aware `containerEngine` validation in full CI runs.
- `pkg/containerd/service_test.go`: reconfigure tests keep synthetic runtime alive through stability window.

### Added — Edgelet Plan 1 (Foundation)

Greenfield rebrand from ioFog Agent to **Edgelet** (`github.com/datasance/edgelet`):

- **Module & toolchain:** Go 1.26.2; containerd **v2.2.3-k3s1** (k3s fork) with cri-api **v1.36.1-k3s1** replace block.
- **Identity:** Labels `edgelet.iofog.org/*`, env `EDGELET_*`, JWT/DNS `edgelet.default.svc.bridge.local`, RBAC group **`edgelet.iofog.org/v1`** only.
- **Local API v1:** All routes moved from `/v3/` to **`/v1/`**; OpenAPI spec at `docs/localapi-v1-openapi.yaml`.
- **Paths & engine:** `/var/lib/edgelet`, `/run/edgelet`, `containerEngine=edgelet` (full flavor); child flag **`--edgelet-containerd-child`**.
- **Deploy manifests:** Accept **`apiVersion: edgelet.iofog.org/v1`** only (kinds unchanged).
- **Provision:** Full profile sends `engine=edgelet` to Pot controller (REST `/api/v3/…` unchanged).

Legacy `cmd/iofog-agent` / `cmd/iofog-agentd` entrypoints retained until Plan 3 multicall merge.

### Changed

- **Provision payload:** Controller provision requests now include `engine` (configured `containerEngine`) and `flavor` (daemon build metadata: `lite` or `full`) alongside the existing `key` and architecture `type` fields. Invalid engine/flavor combinations are rejected before the controller call.

### Changed - CLI operator UX

Consolidated improvements to the Cobra-based `iofog-agent` CLI (help, flags, progress, and mutation output). See [docs/cli/README.md](docs/cli/README.md) and [docs/cli/migration-from-legacy-cli.md](docs/cli/migration-from-legacy-cli.md).

- **Help:** Custom inherited `HelpTemplate` renders `Long`, `Examples`, and Cobra-native flags in terminal `--help` (fixes truncated help from bare `Usage()`). Rich help text for `deploy`, `provision`/`deprovision`, command groups (`system`, `ms`, `image`, `registry`, `runtimeclass`, `auth`), and dangerous-command warnings (`deprovision`, `system stop`, `ms kill`, `ms rm`). Regenerated `docs/cli/generated/` via `make cli-docs`.
- **Flags:** Migrated manual parsers to Cobra flags — `system logs` / `ms logs` (`--follow`, `--tail`, `--since`, `--until`, `--timestamps`), `image load -f`, `deprovision --scope` / `--keep-local`, `ms ls --source`, `ms inspect --summary`, `registry inspect --password-plain`. Fixed `Use` strings on leaf commands; `image pull` adds `-r`/`-p` shorthands.
- **Spinners:** Human-mode mutations show a stderr spinner during long operations (`provision`, `deprovision`, `system stop`/`reload`, `ms` lifecycle, `image load`/`rm`/`pull`, `registry rm`, `runtimeclass rm`, deploy registry apply). Structured `-o json|yaml` skips stderr progress UX.
- **Hybrid mutation output:** `ms start`/`stop`/`restart`/`kill`/`rm`, `provision`, and `deprovision` use a hybrid success pattern — `✔` one-line summary on stderr, multi-line detail on stdout when present (single-line success leaves stdout empty). Mirrors the existing `config` stderr UX contract.

### Fixed

- **Full flavor (CRI/iofog engine):** `ms restart`, `ms start` after stop, and `stop` + `start` now succeed synchronously via remove+create+start instead of failing with `CONTAINER_EXITED` / non-restartable errors. Docker/Podman in-place restart behavior is unchanged. Local reconcile also recreates on `exiting` + `CONTAINER_EXITED`.

### Changed - CLI redesign (breaking)

The `iofog-agent` CLI was rebuilt on Cobra with layered packages (`internal/cli/cmd`, `domain`, `client`, `ui`, `output`). **There are no legacy aliases.** See [docs/cli/migration-from-legacy-cli.md](docs/cli/migration-from-legacy-cli.md) for operator migration steps.

| Legacy | Replacement |
|--------|-------------|
| `iofog-agent status` | `iofog-agent system status` |
| `iofog-agent info` | `iofog-agent system info` |
| `iofog-agent version` | `iofog-agent --version` or `iofog-agent system version` |
| `iofog-agent stop` | `iofog-agent system stop` |
| `iofog-agent prune` | `iofog-agent system prune` |
| `iofog-agent start` | **Removed** — use `iofog-agentd` / `systemctl start iofog-agentd` |
| `iofog-agent cert` | `iofog-agent config cert` |
| `iofog-agent switch` | `iofog-agent config switch` |
| `iofog-agent ms ps` | `iofog-agent ms ls` |
| `iofog-agent deploy apply -f` | `iofog-agent deploy -f` |
| `iofog-agent deploy validate -f` | `iofog-agent deploy -f --dry-run` |
| `iofog-agent deploy registry\|runtimeclass -f` | `iofog-agent deploy -f` (auto kind-detect) |
| `iofog-agent config set KEY VALUE` | `iofog-agent config --key value` (long or `--alias` flags) |

Other breaking behavior:

- Daemon unreachable → exit code **10** (`DAEMON_UNAVAILABLE`), not silent success
- `ms logs --follow` and `ms exec` are human/raw only (`-o json|yaml` rejected)
- Monolithic `internal/cli/commands.go` and `HandleCommand()` removed
- Build metadata injected via `-ldflags` into `internal/cli/cmd` (`Version`, `BuildTime`, `GitCommit`)

### Added - CLI redesign

- Global flags: `-o human|json|yaml`, `--quiet`, `--verbose`, `--debug`, `--socket`, `--timeout`, `--no-color`
- Structured output for data commands; progress/spinners on stderr with `\r\x1b[K` (fixes `(pulling)ng)` corruption)
- Shared async poller for deploy apply, runtimeclass apply, and image pull
- WebSocket transport refactor for `ms logs` / `ms exec` (`internal/cli/client/transport.go`)
- Shell completion: `iofog-agent completion bash|zsh|fish` (hidden)
- Doc generation: `iofog-agent documentation generate md|man` (hidden); `make cli-docs`, `make cli-completion`
- Exit code mapping via typed `CLIError` / `ExitCoder` (including remote exec exit codes)

### Added - CLI documentation and CI (Phase 5)

- `docs/cli/README.md` — flags, exit codes, examples
- `docs/cli/output-schemas.md` — JSON shapes aligned with golden fixtures
- `docs/cli/generated/` — Cobra-generated command reference (`make cli-docs`)
- `make cli-docs-check` — CI drift gate for generated docs
- CLI smoke tests: jq-friendly JSON, exit 10, legacy command rejection suite
- Updated `docs/localapi-v3-qa-gates.md` Gate 5 with automated CLI checks

### Added - Agent 11: Integration, Testing & Finalization
- Integration test suite (Docker, Controller, E2E tests)
- Performance profiling script
- Enhanced Makefile with additional targets (test-unit, test-integration, benchmark, profile, build-size, security-audit)
- Comprehensive documentation:
  - API documentation
  - Architecture documentation
  - Migration guide (Java to Go)
  - Deployment guide
  - Troubleshooting guide
  - Feature parity checklist
- Packaging support:
  - DEB package build scripts
  - RPM package spec file
  - systemd service file
- CI/CD pipelines:
  - Continuous integration workflow
  - Multi-architecture build workflow
  - Release workflow
- Security audit script
- Feature parity validation

### Migration Status
- Agent 1: Repository Structure & Build System ✅
- Agent 2: Core Data Structures & Models ✅
- Agent 3: Configuration & Utilities ✅
- Agent 4: Authentication & Security ✅
- Agent 5: Field Agent ✅
- Agent 6: Process Manager ✅
- Agent 7: Message Bus ✅
- Agent 8: Local API ✅
- Agent 8.1: Additional Modules ✅
- Agent 9: Resource Management & Status Reporting ✅
- Agent 10: Supervisor & CLI ✅
- Agent 11: Integration, Testing & Finalization ✅

### Performance Targets
- Binary size: < 50MB (vs ~200MB with JRE) ✅
- Memory at idle: < 100MB (vs ~300MB Java) ✅
- Startup time: < 2 seconds (vs ~5 seconds Java) ✅
- CPU overhead: < 1% at idle ✅

### Added - Previous Agents
- Initial Go repository structure
- Build system (Makefile)
- Two binaries: `iofog-agent` and `iofog-agentd`
- Dockerfiles (production and development)
- Build scripts
- Documentation (README, CONTRIBUTING, CHANGELOG)
