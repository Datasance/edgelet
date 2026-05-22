# Changelog

All notable changes to the ioFog Agent Go implementation will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
| `iofog-agent config set KEY VALUE` | `iofog-agent config KEY VALUE` |

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
