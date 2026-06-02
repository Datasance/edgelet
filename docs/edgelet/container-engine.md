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
    dockerUrl: unix:///run/edgelet/containerd.sock
```

---

## Engine selection

| Value | Platform | Socket / backend |
|-------|----------|------------------|
| `edgelet` | **linux only** | Embedded containerd at `/run/edgelet/containerd.sock` |
| `docker` | linux, darwin, windows | Host Docker (`dockerUrl`, e.g. `unix:///var/run/docker.sock`) |
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
| `/var/lib/edgelet/` | User data (`diskDirectory`) |
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

Apply via CLI:

```bash
edgelet deploy -f runtimeclass.yaml
edgelet deploy -f runtimeclass.yaml --dry-run
```

Unsupported when `containerEngine` is docker/podman:

`Error[INVALID_ARGUMENT]: runtimeclass is supported only when containerEngine=edgelet`

RBAC and endpoints: [edgelet-api-v1-rbac-resources.md](edgelet-api-v1-rbac-resources.md).

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

Bridge DNS and remediation policies (embedded engine): [../embedded-dns-runbook.md](../embedded-dns-runbook.md), [../embedded-dns-cross-engine-policy.md](../embedded-dns-cross-engine-policy.md).

Engine change lifecycle (cold/warm reload, restart required): [container-engine-lifecycle.md](container-engine-lifecycle.md).
