# Deploy manifest reference

Edgelet accepts **`edgelet.iofog.org/v1`** YAML manifests via:

```bash
edgelet deploy -f manifest.yaml
edgelet deploy -f manifest.yaml --timeout=20m   # ControlPlane async apply
```

Validation runs in the daemon before apply. Shapes are defined in `internal/models/` and surfaced through EdgeletAPI `/v1/deploy/*`.

**Example files:** [examples/](examples/)

| Kind | Example | Guide section |
|------|---------|---------------|
| Microservice | [examples/microservice.yaml](examples/microservice.yaml) | [Microservice](#microservice) |
| Registry | [examples/registry.yaml](examples/registry.yaml) | [Registry](#registry) |
| RuntimeClass | [examples/runtimeclass.yaml](examples/runtimeclass.yaml), [examples/runtimeclass-edgelet-wasmtime.yaml](examples/runtimeclass-edgelet-wasmtime.yaml) | [RuntimeClass](#runtimeclass) |
| ControlPlane | [examples/controlplane.yaml](examples/controlplane.yaml) | [ControlPlane](#controlplane) |

---

## Common fields

| Field | Value |
|-------|--------|
| `apiVersion` | **`edgelet.iofog.org/v1`** (required) |
| `kind` | `Microservice`, `Registry`, `RuntimeClass`, or `ControlPlane` |

Legacy `apiVersion: v3` and Java-era kinds are rejected.

---

## Microservice

Local or operator-managed workload deployed through Edgelet (not Pot controller snapshot).

**Annotated reference:** [examples/microservice.yaml](examples/microservice.yaml) lists every YAML key with inline comments.

### Schema vs implemented

| Field | Status |
|-------|--------|
| `metadata.namespace` | Parsed; **not used** — runtime application is always `edgelet` |
| `spec.config` | Parsed; **not applied** |
| `spec.container.annotations` | Parsed; **not applied** |
| `spec.container.healthCheck` | Parsed; **not wired** in local deploy |
| All other fields in the example | Applied via `BuildMicroserviceFromLocalManifest` |

### Top-level shape

```yaml
apiVersion: edgelet.iofog.org/v1
kind: Microservice
metadata:
  name: <dns-label>           # required
  namespace: edgelet          # optional; use edgelet for local deploy scope
  labels: {}                  # optional user labels (protected keys stripped)
spec:
  image: <image-ref>          # required
  registry: <id>            # optional registry row ID
  container: { ... }          # see below
  schedule: <int>             # optional ordering hint
  config: {}                  # optional opaque config map
```

### `spec.container` (common fields)

| Field | Type | Notes |
|-------|------|-------|
| `hostNetworkMode` | bool | Host network — disables bridge DNS |
| `isPrivileged` | bool | Privileged container |
| `runAsUser` | string | User ID or name |
| `runtime` | string | OCI runtime name (embed engine + RuntimeClass) |
| `platform` | string | Platform selector when pulling |
| `ipcMode`, `pidMode` | string | Passed to engine |
| `capAdd`, `capDrop` | []string | Linux capabilities |
| `env` | `{key,value}[]` | User env (`EDGELET_*` reserved) |
| `extraHosts` | `{name,address}[]` or legacy strings | `/etc/hosts` + docker ExtraHosts |
| `ports` | `{internal,external,protocol}[]` | Port mappings |
| `volumes` | `{hostDestination,containerDestination,accessMode,type}[]` | Bind mounts / volumes |
| `commands` | []string | Container command override |
| `cpuSetCpus` | string | cpuset |
| `memoryLimit` | int64 | Memory limit bytes |
| `healthCheck` | object | Healthcheck spec |

### Apply

```bash
edgelet deploy -f examples/microservice.yaml
edgelet ms ls --source local
```

DNS: [dns.md](dns.md) · Metadata: [workload-metadata.md](workload-metadata.md)

---

## Registry

Credentials for image pulls stored in local SQLite.

```yaml
apiVersion: edgelet.iofog.org/v1
kind: Registry
spec:
  url: <registry-host>        # required
  private: true|false         # required
  username: <string>          # required when private=true
  password: <string>          # required when private=true
  email: <string>             # optional
```

### Apply

```bash
edgelet deploy -f examples/registry.yaml
edgelet registry ls
```

Registry apply is **synchronous**. Secrets are stored locally — treat YAML as sensitive.

---

## RuntimeClass

Maps a **handler name** to OCI runtime configuration on **`containerEngine: edgelet`** (linux embed only).

> **Naming:** `containerEngine: edgelet` is the embedded engine product. WASM workload runtimes use distinct handler keys — for example **`edgelet-wasmtime`** (Datasance shim, `io.containerd.edgelet.v2`) vs upstream **`wasmtime`** (`io.containerd.wasmtime.v1`). See [examples/runtimeclass-edgelet-wasmtime.yaml](examples/runtimeclass-edgelet-wasmtime.yaml).

```yaml
apiVersion: edgelet.iofog.org/v1
kind: RuntimeClass
metadata:
  name: <dns-label>           # required; lowercase DNS label (e.g. edgelet-wasmtime)
handler: <handler>            # required; catalog handler (e.g. edgelet-wasmtime, spin)
```

Reserved name: **`crun`** (built-in default).

### Apply

```bash
edgelet deploy -f examples/runtimeclass.yaml
edgelet runtimeclass ls
```

Reference microservice `spec.container.runtime` to the RuntimeClass name. See [container-engine.md](container-engine.md).

---

## ControlPlane

Deploys **one** Datasance Controller container per Edgelet node (optional — remote `controllerUrl` is valid without local ControlPlane).

**Annotated reference:** [examples/controlplane.yaml](examples/controlplane.yaml) lists every YAML key (active + commented optional blocks).

### Forbidden fields

| Field | Reason |
|-------|--------|
| `metadata.labels` | Rejected at validate |
| `spec.siteCA`, `spec.localCA` | Import via Controller REST after deploy |

```yaml
apiVersion: edgelet.iofog.org/v1
kind: ControlPlane
metadata:
  name: <ms-name>             # required; DNS-1123 label
  namespace: <namespace>      # optional; default applied if empty
spec:
  controller:
    image: <image>            # required
    registry: <id>            # optional
    port: 51121               # optional API port
  auth: { ... }               # optional Keycloak / auth block
  systemMicroservices:        # optional router/nats image maps per arch
    router: { amd64: "...", arm64: "..." }
    nats: { ... }
  nats:
    enabled: true|false
  # events, database, https, vault, ecnViewerPort, logLevel — see control-plane.md
```

### Rules

- **`metadata.labels` forbidden** on ControlPlane manifests.
- **`spec.siteCA` / `spec.localCA` forbidden** — import CAs via Controller REST after deploy.
- At most **one** ControlPlane row per node; delete via `edgelet controlplane delete`.

### Apply (async)

```bash
edgelet deploy -f examples/controlplane.yaml
edgelet controlplane get
```

Default poll budget **15 minutes**. See [control-plane.md](control-plane.md).

### DNS identity

FQDNs derive from `metadata.namespace` + `metadata.name` — see [dns.md](dns.md).

---

## CLI quick reference

| Action | Command |
|--------|---------|
| Apply manifest | `edgelet deploy -f <file>` |
| List local MS | `edgelet ms ls --source local` |
| List registries | `edgelet registry ls` |
| List runtime classes | `edgelet runtimeclass ls` |
| Control plane status | `edgelet controlplane get` |
| Validate only | EdgeletAPI `POST /v1/deploy/microservices:validate` |

---

## Related docs

- [installation.md](installation.md) — install and provisioning
- [deployment.md](deployment.md) — production topology
- [control-plane.md](control-plane.md) — operator guide
- [edgelet-api-v1-openapi.yaml](edgelet-api-v1-openapi.yaml) — HTTP contract
