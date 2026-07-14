# Container engine

Edgelet supports multiple container runtimes through a single `ContainerEngine` interface (`pkg/engine/engine.go`). The process manager, reconciliation loop, and healthcheck runner behave identically regardless of engine. Selection is via `containerEngine` in config, validated against **platform capabilities** (not compile-time flavor).

---

## OS capability matrix

| Platform | Binary | `containerEngine` allowed | Default | Embedded bundle |
|----------|--------|---------------------------|---------|-----------------|
| **linux** | Thin wrapper + fat-in-tar | `edgelet`, `docker`, `podman` | **`edgelet`** | Yes — extract when engine is `edgelet` |
| **darwin** | Monolithic | `docker`, `podman` | `docker` | No |
| **windows** | Monolithic | `docker`, `podman` | `docker` | No |

Factory: `internal/engines/factory_linux.go` / `factory_desktop.go`. Invalid pairings fail at **config** validation (e.g. `edgelet` on darwin/windows).

```yaml
profiles:
  production:
    containerEngine: edgelet
    containerEngineUrl: unix:///run/edgelet/containerd.sock
    pruningFrequency: 24
    watchdogEnabled: true
```

---

## Engine selection

| Value | Platform | Socket / backend |
|-------|----------|------------------|
| `edgelet` | **linux only** | Embedded containerd at `/run/edgelet/containerd.sock` |
| `docker` | linux, darwin, windows | Host Docker (`containerEngineUrl`, e.g. `unix:///var/run/docker.sock`) |
| `podman` | linux, darwin, windows | Host Podman socket |

**No engine fallback:** If docker/podman is configured, Edgelet does not switch to the embedded engine on failure.

---

## Path layout (edgelet engine)

Isolated from host Docker/Podman installations:

| Path | Purpose |
|------|---------|
| `/usr/local/bin/edgelet` | **Thin** download binary (linux): CLI + embed; systemd entry |
| `/var/lib/edgelet/data/current/bin/edgelet` | **Fat** runtime ELF (linux): daemon + in-process containerd |
| `/var/lib/edgelet/data/current/bin/` | Shim (`containerd-shim-runc-v2`), `crun`, CNI multicall + symlinks |
| `/var/lib/edgelet/data/current` / `previous` | Symlinks to active / prior extracted bundle directories |
| `/var/lib/edgelet/` | User data (`diskDirectory`); workload volume data under `volumes/data/` — see [volumes.md](volumes.md) |
| `/var/lib/edgelet-containerd/` | Containerd images, snapshots, CNI state |
| `/run/edgelet/containerd.sock` | Containerd API socket |
| `/run/edgelet/edgelet.sock` | EdgeletAPI Unix socket |

Private bridge network: `edgelet0` (CIDR `172.18.0.0/16`). Container name prefix: `edgelet_`.

---

## Embedded containerd (linux, `containerEngine: edgelet`)

Production starts via **`/usr/local/bin/edgelet daemon`** (thin). The thin process extracts the zstd bundle when needed, then execs **`/var/lib/edgelet/data/current/bin/edgelet daemon`** (fat). The fat binary runs the supervisor and in-process containerd. The containerd child process is spawned with `--edgelet-containerd-child` from the **fat** path only (not from the thin wrapper).

Bundled runtimes (inside the extracted `bin/` directory):

- `containerd-shim-runc-v2` — OCI shim
- `crun` — default low-level runtime
- CNI plugins: `bridge`, `host-local`, `portmap`, `loopback`

Containerd config roots (typical):

```toml
root   = "/var/lib/edgelet-containerd/root"
state  = "/var/lib/edgelet-containerd/state"
address = "/run/edgelet/containerd.sock"
```

CNI conflist: `/var/lib/edgelet-containerd/cni/conf/10-edgelet.conflist`

On data-plane bootstrap (`edgelet runtime-bootstrap` or monolithic embedded start), Edgelet prepares the runtime in order: stop orphaned containerd children, reap managed shims for the edgelet socket, then remove stale runtime task directories under the state tree (`io.containerd.runtime.v2.task/`). Orphaned task directories missing a valid `address` file are removed; `EBUSY` removals are retried and logged without failing bootstrap. Image cache under `/var/lib/edgelet-containerd/root` is preserved. Full state wipe (`CleanupRuntimeArtifacts`) runs only when embedded containerd fails to start, bootstrap retries, and shim reap reports zero remaining PIDs.

---

## Docker and Podman (linux + desktop)

Connect to an external engine. Docker/Podman support native OCI `HEALTHCHECK`; the in-agent healthcheck runner is **edgelet engine only**.

On linux, use systemd `After=docker.service` (or podman) when relying on an external engine at boot. The daemon retries socket connection before reporting engine-ready status.

Manual lifecycle (`edgelet ms start`, `stop`, `restart`) behavior may differ per engine — see OpenAPI notes for engine-specific semantics.

---

## RuntimeClass (edgelet engine only)

Runtime extensions use EdgeletAPI deploy manifests:

- `apiVersion: edgelet.iofog.org/v1`
- `kind: RuntimeClass`
- fields: `metadata.name`, `handler`

Each `metadata.name` registers one canonical runtime handler. Network scope (`managed` vs `local`) is selected via workload CNI policy, not by synthesizing handler variants.

Built-in catalog handlers include `spin`, `edgelet-wasmtime` (Datasance WASM shim), and upstream `wasmtime` / `wasmedge` when the matching shim binary is on `PATH`. Shim binaries are discovered from `PATH` (for example `containerd-shim-edgelet-v2` for handler `edgelet-wasmtime`).

Example WASM RuntimeClass:

```yaml
apiVersion: edgelet.iofog.org/v1
kind: RuntimeClass
metadata:
  name: edgelet-wasmtime
handler: edgelet-wasmtime
```

Pin a microservice: `spec.container.runtime: edgelet-wasmtime` (references `metadata.name`, not `containerEngine`).

Apply via CLI:

```bash
edgelet deploy -f runtimeclass.yaml
edgelet deploy -f runtimeclass.yaml --dry-run
```

Unsupported when `containerEngine` is docker/podman:

`Error[INVALID_ARGUMENT]: runtimeclass is supported only when containerEngine=edgelet`

RBAC and endpoints: [edgelet-api-v1-rbac-resources.md](edgelet-api-v1-rbac-resources.md).

---

## Data-plane restart and shim upgrades (embedded)

**Safe `edgelet-containerd` restart:** prefer **`stop` then `start`** over blind `restart` during shim or catalog upgrades. A stop/start cycle lets `runtime-bootstrap` drain MS cleanly and avoids racing extract/rename on a warm bundle dir.

```bash
sudo systemctl stop edgelet-containerd
sudo systemctl start edgelet-containerd
sudo journalctl -u edgelet-containerd -n 20 --no-pager   # Embedded containerd is ready
```

OpenRC: `rc-service edgelet-containerd stop` then `rc-service edgelet-containerd start`.

**Shim upgrade sequence** (no `edgelet config` reconfigure required):

1. `systemctl stop edgelet` (control only — MS keep running on data plane)
2. Install new shim binaries into the active bundle `bin/` (OTA or manual copy)
3. `systemctl stop edgelet-containerd` then `systemctl start edgelet-containerd`
4. `systemctl start edgelet`

Catalog runtimes registered in `/var/lib/edgelet-containerd/config.toml` pick up new shims on `PATH` only after a **data-plane restart**. Control-plane restart alone is not enough.

Crash-loop symptoms (`rename extracted bundle: file exists`, repeated `Preparing data dir`): [troubleshooting.md](troubleshooting.md#embed-bundle--data-plane-restart).

Orphan shim recovery after a failed data-plane stop: [troubleshooting.md](troubleshooting.md#catalog-runtime--orphan-shims-after-data-plane-restart).

---

## ContainerEngine interface

Every engine implements pull/create/start/stop/remove, image management, exec sessions, drift detection, and network ensure. Optional `HealthcheckEngine` (`ExecWithExitCode`) is implemented by the edgelet adapter only.

Implementation packages:

| Engine | Package |
|--------|---------|
| `edgelet` | `pkg/engine/edgelet/` |
| `docker` | `pkg/engine/docker/` |
| `podman` | `pkg/engine/podman/` |

In-process containerd service: `pkg/containerd/`.

---

## DNS and cross-engine policy

Bridge DNS (all engines): [dns.md](dns.md). Workload labels: [workload-metadata.md](workload-metadata.md).

Engine change lifecycle (cold/warm reload, restart required): [container-engine-lifecycle.md](container-engine-lifecycle.md).
