# Workload metadata (labels and environment)

Edgelet stamps every container with a **canonical metadata contract** so identity, ownership, and selectors work the same across **`edgelet`**, **docker**, and **podman** engines.

**Source of truth:** `internal/workloadmeta/` (`spec.go`, `build.go`).

---

## Overview

| Item | Value |
|------|--------|
| Label namespace | `edgelet.iofog.org/*` (plus standard `app.kubernetes.io/*`) |
| Env prefix | `EDGELET_*` |
| Managed-by | `app.kubernetes.io/managed-by: edgelet` |
| Container name prefix | `edgelet_<uuid>` — debug only; **never** use for identity |

User-supplied labels and env vars are merged in, but **protected keys cannot be overridden**.

---

## LabelSpec

### Required labels (agent-written)

| Label | Value |
|-------|--------|
| `app.kubernetes.io/name` | Microservice name |
| `app.kubernetes.io/instance` | Microservice UUID |
| `app.kubernetes.io/part-of` | Application / namespace name |
| `app.kubernetes.io/managed-by` | `edgelet` |
| `edgelet.iofog.org/microservice-uid` | Microservice UUID |
| `edgelet.iofog.org/node-uid` | Edgelet node UUID (`iofogUuid` in config) |
| `edgelet.iofog.org/scope` | `managed` or `local` |
| `edgelet.iofog.org/runtime-engine` | `edgelet`, `docker`, or `podman` |
| `edgelet.iofog.org/role` | `workload`, `router`, `nats`, `controller`, or `edgelet` |

### Optional labels

| Label | When set |
|-------|----------|
| `edgelet.iofog.org/system` | `true` for system workloads (router, nats, controller, edgelet daemon container) |
| `edgelet.iofog.org/host-network` | `true` when `hostNetworkMode` is enabled |
| `edgelet.iofog.org/sandbox-id` | CRI pod sandbox ID (`containerEngine: edgelet` only) |
| `edgelet.iofog.org/healthcheck` | JSON-encoded healthcheck (`models.Healthcheck`) |

### Normalization

- Label keys are lowercased on merge.
- Booleans serialize as `"true"` / `"false"`.
- Protected labels reject user overrides (see `ProtectedLabelKeys` in `spec.go`).

### Legacy labels (removed)

Runtime code must **not** read or write:

`iofog-ms`, `iofog-name`, `iofog-app`, `iofog-uuid`, `iofog.uuid`, `iofog-router`, `iofog-nats`, `iofog-system`, `iofog-hostnet`, `iofog-sandbox-id`, `iofog-healthcheck`

### Engine-internal operational labels (edgelet engine only)

Non-identity state on containerd workloads (not part of LabelSpec):

- `iofog-ip`, `iofog-netns`, `iofog-started-at`, `iofog-ports`, `iofog-log-size`

---

## EnvSpec

### Required predefined env vars

| Variable | Meaning |
|----------|---------|
| `EDGELET_MICROSERVICE_UID` | Microservice UUID |
| `EDGELET_MICROSERVICE_NAME` | Microservice name |
| `EDGELET_APPLICATION_NAME` | Application / namespace |
| `EDGELET_NODE_UID` | Edgelet node UUID |
| `EDGELET_SCOPE` | `managed` or `local` |
| `EDGELET_RUNTIME_ENGINE` | `edgelet`, `docker`, or `podman` |
| `EDGELET_ROLE` | Role string (see below) |

### TZ policy

- If the user env already contains `TZ`, that value is preserved.
- Otherwise Edgelet injects `TZ` from config (`timeZone`), default **`UTC`**.

### Reserved env vars

The keys above cannot be overridden by user env injection (`ReservedEnvKeys`).

### Legacy env (removed)

- `SELFNAME` — superseded by `EDGELET_MICROSERVICE_UID` and related keys.

---

## Role

Derived with deterministic precedence:

1. **Controller** workloads → `controller`
2. Else if `IsRouter` → `router`
3. Else if `IsNats` → `nats`
4. Else → `workload`

If both router and nats flags are set, **router wins**.

The edgelet daemon container itself uses role **`edgelet`** when labeled by the watchdog path.

---

## Scope

| Condition | Scope |
|-----------|--------|
| `hostNetworkMode: true` | `managed` (host network bypasses bridge scoping) |
| `metadata.namespace` / application is **`edgelet`** (local deploy) | `local` |
| All other managed workloads | `managed` |

Local deploy manifests use `metadata.namespace: edgelet` (see [manifest-reference.md](manifest-reference.md)).

Control plane workloads are treated as **`local`** scope for DNS listener partitioning.

---

## Examples

### Docker / Podman (managed workload)

```yaml
labels:
  app.kubernetes.io/name: video-analyzer
  app.kubernetes.io/instance: 6f2f347f-a43b-43fb-9f72-2f6f47aa91be
  app.kubernetes.io/part-of: smart-city
  app.kubernetes.io/managed-by: edgelet
  edgelet.iofog.org/microservice-uid: 6f2f347f-a43b-43fb-9f72-2f6f47aa91be
  edgelet.iofog.org/node-uid: node-123
  edgelet.iofog.org/scope: managed
  edgelet.iofog.org/runtime-engine: docker
  edgelet.iofog.org/role: workload
  edgelet.iofog.org/system: "false"
  edgelet.iofog.org/host-network: "false"
env:
  - EDGELET_MICROSERVICE_UID=6f2f347f-a43b-43fb-9f72-2f6f47aa91be
  - EDGELET_MICROSERVICE_NAME=video-analyzer
  - EDGELET_APPLICATION_NAME=smart-city
  - EDGELET_NODE_UID=node-123
  - EDGELET_SCOPE=managed
  - EDGELET_RUNTIME_ENGINE=docker
  - EDGELET_ROLE=workload
  - TZ=UTC
```

### Local deploy (`edgelet ms` / `edgelet deploy -f`)

```yaml
labels:
  app.kubernetes.io/part-of: edgelet
  edgelet.iofog.org/scope: local
  edgelet.iofog.org/runtime-engine: edgelet
```

---

## Identity helpers

Code should use:

- `workloadmeta.MicroserviceUIDFromLabels(labels)` — canonical UUID
- `workloadmeta.IsManagedByIofog(labels)` — managed-by + UUID present
- `workloadmeta.ResolveScope(application, hostNetwork)` — scope string

Do **not** parse container names or legacy `iofog-*` labels for identity.

---

## Related docs

- [dns.md](dns.md) — how scope affects DNS listeners and aliases
- [manifest-reference.md](manifest-reference.md) — deploy YAML shapes
- [container-engine.md](container-engine.md) — engine selection
