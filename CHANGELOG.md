# Changelog

All notable changes to Edgelet are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0-rc.1] — mid-June 2026

Release candidate after provision lifecycle hardening, runtime observability fixes, and supply-chain follow-up. Pair with Controller **v3.8.0-beta.1** or newer v3.8 line.

### Added

- **Lifecycle dev gates:** `make pre-it` (vet + lint + lifecycle unit tests); optional `make test-lifecycle-race` and `make test-deadlock` for concurrency audits.
- **Deadlock audit doc:** operator-facing lock inventory and fixes in `docs/edgelet/deadlock-audit.md`.
- **Healthcheck shell helper:** shared `ShellCommandForScript` (bash → sh → busybox) for `CMD-SHELL` healthchecks and default exec sessions on minimal images.

### Changed

- **Live reprovision:** `edgelet system provision` on a running daemon reloads registries, volume mounts, microservices, and config without restarting background workers.
- **Split log basenames:** `edgelet.service` writes `edgelet.*.log`; `edgelet-containerd.service` writes `edgelet-containerd.*.log` under the same `logDiskDirectory` (60/40 rotation budget when runtime split is enabled). See `docs/edgelet/logging.md`.
- **Disk usage metric:** `edgelet system status` `diskUsage` walks the full configured `diskDirectory` instead of legacy `messages/archive` + `volumes/` partial sums; automatic archive prune removed (greenfield).
- **Image drift (embedded engine):** reconcile compares images with `imageref.Match` so controller short names (e.g. `user/repo:tag`) match runtime `docker.io/` prefixes without endless `config_drift` UPDATE loops.
- **Deprovision cleanup:** volume-mount index cleanup no longer holds locks across slow filesystem work that blocked the process-manager monitor during background deprovision.
- **Changes worker resilience:** panic recovery and per-cycle timeout so `config/changes` polling continues at `changeFrequency` after handler failures.
- **Go dependencies:** minor bumps — `containerd/typeurl/v2` 2.3.0, `fsnotify` 1.10.1, `docker/go-connections` 0.7.0, `golang.org/x/crypto` 0.53.0, `pflag` 1.0.10.
- **Supply-chain CI:** SHA-pinned GitHub Actions bumps (checkout v6, docker setup-buildx/qemu v4, action-gh-release v3, cosign-installer 4.1.2) across ci, release, codeql, govulncheck, and scorecard workflows.
- **CI lint:** `quality-linux.sh` runs vet plus a single full golangci pass; CI lint job aligned (no redundant staticcheck chain).

### Fixed

- **Volume mount deadlock:** CONFIGMAP/SECRET version bumps during `config/changes` no longer re-acquire `indexLock` and freeze the changes worker.
- **Live reprovision gap:** reprovision without daemon restart now loads controller microservices (same path as cold `Start()`).
- **Deprovision monitor stall:** process-manager container monitoring continues through deprovision volume cleanup.
- **False-positive image drift:** embedded-engine reconcile no longer recreates microservices when only the Docker Hub hostname prefix differs.
- **Misleading disk metric:** status `diskUsage` now reflects deployed workload data under `/var/lib/edgelet/` when volumes and MS data are present.
- **Log file confusion:** control-plane traffic lands in `edgelet.0.log` instead of being rotated into secondary files while `edgelet-containerd` bootstrap stays in its own series.
- **`CMD-SHELL` healthchecks:** succeed on images with bash but no `/bin/sh` when bash is present.

### Known limitations (rc)

- **Pre-release:** `v1.0.0-rc.1` is a release candidate, not production GA.
- **Host-network DNS:** `edgelet.default.svc.bridge.local` injection remains an accepted limitation for host-network microservices.
- **Provisioned guard IT:** blocked `ms rm` / `controlplane delete` when provisioned still deferred (requires live system fog).

## [1.0.0-beta.3] — mid-June 2026

Production hardening release paired with Controller **v3.8.0-beta.1**. Deploy Edgelet and Controller v3.8 together; greenfield ControlPlane YAML only.

### Added

- **Controller microservice register-once:** after provision and local ControlPlane deploy, Edgelet calls `POST /api/v3/agent/controller/register` once and retries until success; no re-register on spec drift.
- **OTA controller semver:** `GET /api/v3/agent/version` `semver` field normalized to `vX.Y.Z` for `install.sh`; `provisionKey` and `expirationTime` (Unix ms) drive post-OTA reprovision (private key rotation only; stable `iofogUuid`).
- **Per-OS install paths:** documented linux, darwin, and windows directory tables in README and `docs/edgelet/installation.md`; embedded `uninstall.sh` in the install monolith; linux `/usr/share/edgelet/` ships both scripts after curl-only install.

### Changed

- **ControlPlane manifest v3.8:** OIDC auth, EdgeOps Console, TLS, and `controller.publicUrl` / `trustProxy`; canonical env projection (`AUTH_*`, `OIDC_*`, `CONSOLE_*`, `TLS_*`, `INTERMEDIATE_CERT`); host ports **51121** (API) and **80** → console.
- **Config hot-reload:** PATCH `/v1/config` and `edgelet system reload` apply log level without service restart; shared reload path for SIGHUP, PATCH, and POST reload; `logging.InstanceConfigUpdated` on hot reload.
- **Moby SDK (docker/podman engine):** replaced legacy `github.com/docker/docker` with `github.com/moby/moby/client@v0.4.1` and `github.com/moby/moby/api@v1.54.2`; removed govulncheck exceptions **GO-2026-4887** / **GO-2026-4883**.
- **`edgelet system reload` UX:** human success output (spinner model) when not using structured `-o`.

### Fixed

- **Curl-only linux install:** `/usr/share/edgelet/uninstall.sh` now present after pipe-to-bash install (embedded in assemble pipeline).
- **macOS install:** darwin uses `/var/run/edgelet` (not `/run`) and `/usr/local/share/edgelet/` for bundled scripts; avoids failures on default macOS layout.

### Breaking

- **Legacy ControlPlane YAML rejected:** keys such as `auth.url`, `ecnViewer*`, `spec.https`, Keycloak/viewer/ssl-era fields fail validation; use v3.8 schema and examples under `docs/edgelet/examples/controlplane.yaml`.

### Known limitations (beta)

- **Pre-release:** `v1.0.0-beta.3` is not production GA; pair with Controller **3.8.0-beta.1** or newer v3.8 line.
- **Provisioned guard IT:** blocked `ms rm` / `controlplane delete` when provisioned deferred (requires live system fog); unprovisioned lifecycle covered in CP IT.

## [1.0.0-beta.2] — mid-June 2026

### Fixed

- **Cgroup preflight on LXC/VM machine roots:** OpenRC hosts with `/.lxc` cgroup layout (e.g. OrbStack Alpine) no longer fail cold boot with a misleading docker `--privileged` error. Machine-root paths are distinguished from workload-nested containers; `edgelet cgroup-preflight` performs light mount/mode checks only; strict delegation validation runs after bootstrap prep.
- **LXC/VM machine OpenRC containerd restart:** `/.lxc/init` staging cgroups are treated as machine-root boundaries (not workload-nested). Runtime bootstrap skips Moby-style reparent when `edgelet-cgroup-prep` already delegated at the unified root and `/.lxc`, then prepares the OpenRC/service staging cgroup and `/edgelet` before spawning embedded containerd (OrbStack Alpine cold boot).
- **OpenRC sysinit prep:** `edgelet-cgroup-prep` reparents unified root and `/.lxc` cgroups and delegates `cpu`/`memory`/`pids` before `edgelet-containerd` starts (Moby/dind-style).

### Changed

- **`cgroupNested` status:** `true` only for workload containers (Docker/k8s dev deploy), not LXC/VM machine boundaries.

## [1.0.0-beta.1] — mid-June 2026
Hotfix pre-release after `v1.0.0-beta.0`. GitHub Releases remain binary-only; `install.sh` is now self-contained.
### Fixed
- **Curl / fresh-host install:** `install.sh` no longer requires co-located `scripts/lib/` or `packaging/init/` on the release download path. A single `install.sh` from GitHub Releases embeds init helpers and unit templates (assemble pipeline); fixes `Missing init helper scripts` on first install.
- **Post-install layout:** `/usr/share/edgelet/` ships self-contained `install.sh` / `uninstall.sh` and config samples only — not `lib/` or `init/` trees.
### Changed
- **Supply-chain CI:** Dependabot (gomod + github-actions), CodeQL for Go, pinned Actions and Docker base images, `read-all` workflow defaults, keyless cosign signatures on release assets, bounded fuzz smoke in CI, `SCORECARD_TOKEN` for OpenSSF Scorecard branch-protection checks.
- **Dependencies:** `golang.org/x/net` bumped (vulnerability reduction).
### Known limitations (beta)
- **Pre-release:** `v1.0.0-beta.1` is not production GA; expect refinements before 1.0.0.

## [1.0.0-beta.0] — mid-June 2026

First public pre-release of **Edgelet** — a greenfield edge runtime and ioFog/PoT node agent (`github.com/eclipse-iofog/edgelet`). GitHub Releases are marked **Pre-release** and ship binary-only artifacts (no DEB/RPM, no release tarballs).

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
- **Dual publish:** Eclipse upstream at [eclipse-iofog/edgelet](https://github.com/eclipse-iofog/edgelet); Datasance mirror at [Datasance/edgelet](https://github.com/Datasance/edgelet) with identical tags (separate GHCR namespaces).
- **Container image:** `ghcr.io/eclipse-iofog/edgelet:<tag>` (scratch base, `EDGELET_DAEMON=container`); Datasance mirror: `ghcr.io/datasance/edgelet:<tag>`.
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