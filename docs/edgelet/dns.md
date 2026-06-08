# DNS and service discovery

Edgelet provides **bridge-network service discovery** for microservices on a single node. Behavior depends on `containerEngine` in config.

| Engine | Mechanism | Authoritative resolver |
|--------|-----------|------------------------|
| **`edgelet`** (linux embed) | Embedded DNS on bridge gateway + per-container `resolv.conf` / `/etc/hosts` | **Yes** — `internal/dnsresolver` |
| **`docker`** | User-defined bridge **`edgelet`** + network **aliases** + **`ExtraHosts`** | **No** |
| **`podman`** | Same as docker (shared client) | **No** |

**Zone:** `svc.bridge.local` (default).

**Bridge:** Linux bridge **`edgelet0`**, CIDR **`172.18.0.0/16`**, Docker/Podman network name **`edgelet`**.

See also: [container-engine.md](container-engine.md), [workload-metadata.md](workload-metadata.md), [control-plane.md](control-plane.md).

---

## When DNS applies

| Mode | Bridge | Service discovery |
|------|--------|-------------------|
| `hostNetworkMode: false` | Workload joins `edgelet` bridge | Aliases + (embed) resolver records |
| `hostNetworkMode: true` | Host network | No bridge aliases; no embed `resolv.conf` mount |

Host-network workloads use the host resolver directly.

---

## FQDN catalog

### Reserved system names (managed scope)

| FQDN | Role |
|------|------|
| `edgelet.default.svc.bridge.local` | Edgelet node agent (bridge gateway) |
| `router.default.svc.bridge.local` | Router microservice |
| `nats.default.svc.bridge.local` | NATS microservice |

### Per-workload names

For application **`myapp`** and microservice **`worker`**:

| Form | Example |
|------|---------|
| Short (bridge alias / in-zone) | `myapp.worker` |
| FQDN | `myapp.worker.svc.bridge.local` |
| UUID alias | `iofog_<uuid>` and `iofog_<uuid>.svc.bridge.local` |

The `iofog_` prefix on UUID aliases is a **stable token** in resolver code; it is not related to legacy product naming elsewhere.

### Control plane (three FQDNs)

For `metadata.namespace` = **`default`**, `metadata.name` = **`pot`**:

| # | FQDN |
|---|------|
| 1 | `edgelet.controller.svc.bridge.local` |
| 2 | `controller.default.svc.bridge.local` |
| 3 | `default.pot.svc.bridge.local` |

Identity comes from manifest metadata — not hardcoded `"controller"` as the microservice name. See [control-plane.md](control-plane.md).

### Compatibility aliases (optional, embed engine)

When enabled, the embedded resolver also publishes:

- `host.docker.internal`
- `host.container.internal`

Controlled by resolver config (`dnsCompatAliasesEnabled` in status).

---

## Embedded engine (`containerEngine: edgelet`)

Linux only. Started from `pkg/engine/edgelet` when the CRI engine creates workloads.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Bridge edgelet0 (172.18.0.0/16)                       │
│  Gateway .1 ──► embedded DNS :53 (managed + local)      │
│                                                          │
│  Pod sandboxes ──► resolv.conf → gateway                 │
│                 └── /etc/hosts (extraHosts + baseline)   │
└─────────────────────────────────────────────────────────┘
         │
         │ out-of-zone queries
         ▼
   host /etc/resolv.conf upstreams
```

### Scopes (listener partitioning)

Internal scope constants (`internal/dnsresolver/resolver.go`):

| Scope constant | Label scope | Listener |
|----------------|-------------|----------|
| `edgelet` (managed) | `managed` workloads | Bridge gateway `:53` |
| `iofog-local` (local) | `local` workloads (`namespace: edgelet`) | Separate bind address on same bridge |

Reconcile loop pulls live container state from containerd (`runtimeDNSSnapshot`) and upserts `WorkloadRecord` entries. Control plane rows are also upserted via `internal/processmanager/controlplane_dns.go`.

### Record publication (`aliasesForWorkload`)

For each active workload IP, the resolver answers A/AAAA for:

- `<app>.<name>` (+ FQDN)
- `iofog_<uuid>` (+ FQDN)
- Control-plane extras when `IsController`
- Reserved router/nats/agent names on **managed** scope
- Optional docker compat hostnames

Inactive containers may return `NXDOMAIN` or policy denial depending on query type.

### Per-container files (CRI create path)

Non-host-network pods:

1. **`resolv.conf`** — nameserver = bridge gateway IP for workload scope
2. **`/etc/hosts`** — user `extraHosts` plus baseline entries

Paths under Edgelet state dirs (see `pkg/engine/edgelet/engine.go`).

### Forwarding

Queries outside `svc.bridge.local` forward to upstreams parsed from the **host** `/etc/resolv.conf`, with backoff and health tracking (`internal/dnsresolver/forwarding.go`).

### Persistence

DNS workload snapshot: `/var/lib/edgelet/dns/snapshot-v1.json` (restore on startup).

### Observability

When `containerEngine: edgelet`, `GET /v1/system/status` includes DNS fields:

| Key | Meaning |
|-----|---------|
| `dnsStarted` | Resolver running |
| `dnsScopeManagedListening` / `dnsScopeManagedAddress` | Managed listener |
| `dnsQueriesTotal`, `dnsSuccessTotal`, `dnsNXDomainTotal`, `dnsServFailTotal` | Query outcomes |
| `dnsForwardedTotal`, `dnsForwardErrTotal`, `dnsForwardingDegraded` | Upstream forwarding |
| `dnsHealth` | Derived health (`ok`, `degraded`, `stopped`, …) |

CLI: `edgelet system status` (human or `-o json`).

---

## Docker and Podman

External engines do **not** start the embedded resolver. Discovery uses Docker networking primitives on the shared **`edgelet`** bridge.

### Network selection

All non-host workloads attach to network **`edgelet`** (`pkg/docker/network.go`). Application name does not select a different bridge — scope is metadata-only.

Edgelet ensures the network exists at container create time.

### Network DNS aliases

Short names registered on the bridge endpoint (`NetworkingConfig.EndpointsConfig`):

```go
dnsresolver.WorkloadBridgeNetworkAliases(application, name, isController)
```

| Workload | Aliases (short) |
|----------|-----------------|
| General | `<application>.<name>` |
| Control plane | above + `edgelet.controller`, `controller.<namespace>` |

These resolve via **Docker's embedded DNS** on the `edgelet` network, not via `svc.bridge.local` unless the querying container uses FQDNs in `/etc/hosts`.

### ExtraHosts (`/etc/hosts`)

Docker path builds `hostConfig.ExtraHosts`:

1. **`edgelet.default.svc.bridge.local:<hostIP>`** — prepended unless user already mapped it (`buildExtraHostsWithIoFog`)
2. **`router.default.svc.bridge.local:<routerIP>`** — when router IP known and workload is not the router
3. **`nats.default.svc.bridge.local:<natsIP>`** — when NATS IP known
4. User **`extraHosts`** from manifest (`name:address` or YAML struct)

Host-network mode: **no** `ExtraHosts`, **no** bridge network — `NetworkMode: host`.

### Podman

Podman uses the same docker-compatible client code paths (`RuntimeEnginePodman` in labels only).

---

## Cross-engine parity rules

1. **Same bridge name** (`edgelet`) for all non-host workloads on docker/podman.
2. **Same alias function** for docker/podman network aliases and embed resolver short names (`application.name`).
3. **Same reserved FQDN strings** in ExtraHosts and embed resolver.
4. **Scope** is label/env metadata — not a separate bridge per scope on docker/podman.

Drift detection on docker compares expected network + alias policy against runtime inspect (see process manager reconcile).

---

## Troubleshooting

| Symptom | Checks |
|---------|--------|
| Name does not resolve (embed) | `edgelet system status` → `dnsStarted`, `dnsHealth`; verify workload not host-network |
| Name does not resolve (docker) | `docker network inspect edgelet`; container on network; aliases in inspect |
| Router/NATS unreachable | System MS running; ExtraHosts populated; embed reserved records present |
| External name fails (embed) | `dnsForwardingDegraded`; host `/etc/resolv.conf` upstreams |
| Control plane DNS wrong | Confirm `metadata.namespace` / `metadata.name`; see FQDN table above |

More: [troubleshooting.md](troubleshooting.md).

---

## Implementation map

| Component | Package / file |
|-----------|----------------|
| Embedded resolver | `internal/dnsresolver/` |
| CRI hooks (resolv, hosts) | `pkg/engine/edgelet/engine.go` |
| Docker aliases + ExtraHosts | `pkg/docker/container.go` |
| Bridge network ensure | `pkg/docker/network.go`, `internal/network/` |
| Control plane DNS upsert | `internal/processmanager/controlplane_dns.go` |
| Label-driven scope | `internal/workloadmeta/` |
