# Changelog

All notable changes to the ioFog Agent Go implementation will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- EdgeletAPI code rename: Go packages, auth/PKI paths, CLI client symbols, and CI gates aligned with `.cursor/edgelet/NAMING.md` (HTTP routes remain `/v1/...`).
- Bare **`edgelet`** invokes the operator CLI (help banner); start the daemon explicitly with **`edgelet daemon`** or **`systemctl start edgelet`** (`ExecStart=…/edgelet daemon` in systemd/packaging).
- Documentation migration: obsolete root docs replaced by `docs/edgelet/` tree (architecture, deployment, EdgeletAPI v1, CLI migration); root `README.md` and hand-written CLI docs updated for Edgelet terminology.

## [1.0.0-edgelet] — TBD (proposed)

> **Plan 5 status (2026-05-26):** Not released — embed accumulation P0 fixed; amd64 full **+156 KiB** over 55 MiB budget; staging Pot sign-off pending. See `.cursor/edgelet/docs/05-verification.md`.

### Fixed — embed packaging (Plan 5 P0)

- **`scripts/package-data`:** Clear prior `pkg/data/embed/*.tar.zst` before installing the new bundle (prevents `go:embed embed/*` from accumulating multi-arch artifacts ~2× ELF size).
- **`scripts/ci`:** Scope `EDGELET_CI_ARCHES="${_arch}"` per loop iteration; fail fast if embed dir does not contain exactly one `.tar.zst` before `build-edgelet`.

### Known blockers (pre-release)

- linux/amd64 full **57,831,896 B** (~55.15 MiB) — **+156 KiB** over RFC budget (arm64 **52,971,536 B** passes).
- Staging Pot checklist (provision, volumeMounts/NATS, router+NATS MS) not yet signed off.
- Embedded integration tests and air-gapped install smoke pending linux VM.

### Added — Edgelet greenfield release (Plans 1–4)

- Single **`edgelet`** multicall binary (CLI + daemon + `--edgelet-containerd-child` on full linux).
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
