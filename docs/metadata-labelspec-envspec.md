# Metadata LabelSpec and EnvSpec

## Overview

This document defines the canonical container metadata contract for Edgelet.

- Namespace: `iofog.org`
- Runtime scope: Docker, Podman, embedded iofog/containerd
- Compatibility mode: none (legacy keys are removed)

This contract is authoritative for:

- workload identity
- managed/non-managed ownership checks
- selector/search parity across engine types
- predefined runtime environment variables

Container name prefix `iofog_` remains allowed for readability/debugging only and must never be used as an identity source.

## LabelSpec v1 (`iofog.org`)

### Required labels

- `app.kubernetes.io/name`
  - Value: microservice name
- `app.kubernetes.io/instance`
  - Value: microservice UUID
- `app.kubernetes.io/part-of`
  - Value: application name (`local` for local workloads)
- `app.kubernetes.io/managed-by`
  - Value: `edgelet`
- `iofog.org/microservice-uid`
  - Value: microservice UUID
- `iofog.org/node-uid`
  - Value: agent node UUID (`iofogUuid`)
- `iofog.org/scope`
  - Value: `managed` or `local`
- `iofog.org/runtime-engine`
  - Value: `docker`, `podman`, or `iofog`
- `iofog.org/role`
  - Value: `workload`, `router`, or `nats`

### Optional labels

- `iofog.org/system`
  - Value: `true` or `false`
- `iofog.org/host-network`
  - Value: `true` or `false`
- `iofog.org/sandbox-id`
  - Value: sandbox ID (iofog/containerd only)
- `iofog.org/healthcheck`
  - Value: JSON-encoded `models.Healthcheck` (iofog/containerd exec-health path and controller-disconnect resilience)

### Normalization rules

- Label keys are lowercase.
- Boolean values are serialized as string literals: `true` / `false`.
- Values are plain strings. The only structured JSON permitted in LabelSpec v1 is the health payload on **`iofog.org/healthcheck`**.
- Canonical protected labels cannot be overridden by user-supplied labels.

### Non-authoritative metadata

- Container name prefix `iofog_` is non-authoritative and must not be used for:
  - microservice identity extraction
  - managed workload detection
  - selector resolution
  - running workload counts

### Legacy labels removed

These keys must not be read or written by runtime code:

- `iofog-ms`
- `iofog-name`
- `iofog-app`
- `iofog-uuid`
- `iofog.uuid`
- `iofog-router`
- `iofog-nats`
- `iofog-system`
- `iofog-hostnet`
- `iofog-sandbox-id`
- `iofog-healthcheck`

### Engine-internal operational labels (iofog/containerd only)

The embedded engine may still persist **non-identity** operational state on containerd workloads using `iofog-*` keys that are **not** part of LabelSpec v1 and must not be used for identity or cross-engine parity:

- `iofog-ip` — cached pod IP for status and DNS helpers
- `iofog-netns` — network namespace path
- `iofog-started-at` — start timestamp (Unix ms)
- `iofog-ports` — JSON port mappings for drift detection (non-host workloads)
- `iofog-log-size` — log size hint

Workload identity, role, node binding, sandbox ID, host-network mode, healthcheck JSON (`iofog.org/healthcheck`), and managed-by semantics use **only** the canonical LabelSpec keys above.

## EnvSpec v1

### Required predefined env vars

- `IOFOG_MICROSERVICE_UID`
- `IOFOG_MICROSERVICE_NAME`
- `IOFOG_APPLICATION_NAME`
- `IOFOG_NODE_UID`
- `IOFOG_SCOPE`
- `IOFOG_RUNTIME_ENGINE`
- `IOFOG_ROLE`

### TZ policy

- If user env already contains `TZ`, preserve user value.
- Otherwise inject `TZ` from config (`cfg.TimeZone`), with fallback `UTC`.

### Reserved/protected env vars

The following keys are agent-controlled and cannot be overridden by user env injection:

- `IOFOG_MICROSERVICE_UID`
- `IOFOG_MICROSERVICE_NAME`
- `IOFOG_APPLICATION_NAME`
- `IOFOG_NODE_UID`
- `IOFOG_SCOPE`
- `IOFOG_RUNTIME_ENGINE`
- `IOFOG_ROLE`

### Legacy env removed

- `SELFNAME` (superseded by `IOFOG_MICROSERVICE_UID` and related `IOFOG_*` keys)

## Role definition

Role is derived from microservice flags with deterministic precedence:

1. If `IsRouter == true`, role is `router`.
2. Else if `IsNats == true`, role is `nats`.
3. Else role is `workload`.

If both flags are true, precedence applies: `router > nats > workload`.

## Scope definition

Scope is derived as:

- `local` when `ApplicationName == "local"` and `HostNetworkMode == false`
- `managed` otherwise

## Canonical examples (same key set across engines)

### Docker example

```yaml
labels:
  app.kubernetes.io/name: "video-analyzer"
  app.kubernetes.io/instance: "6f2f347f-a43b-43fb-9f72-2f6f47aa91be"
  app.kubernetes.io/part-of: "smart-city"
  app.kubernetes.io/managed-by: "edgelet"
  iofog.org/microservice-uid: "6f2f347f-a43b-43fb-9f72-2f6f47aa91be"
  iofog.org/node-uid: "node-123"
  iofog.org/scope: "managed"
  iofog.org/runtime-engine: "docker"
  iofog.org/role: "workload"
  iofog.org/system: "false"
  iofog.org/host-network: "false"
env:
  - IOFOG_MICROSERVICE_UID=6f2f347f-a43b-43fb-9f72-2f6f47aa91be
  - IOFOG_MICROSERVICE_NAME=video-analyzer
  - IOFOG_APPLICATION_NAME=smart-city
  - IOFOG_NODE_UID=node-123
  - IOFOG_SCOPE=managed
  - IOFOG_RUNTIME_ENGINE=docker
  - IOFOG_ROLE=workload
  - TZ=UTC
```

### Podman example

```yaml
labels:
  app.kubernetes.io/name: "video-analyzer"
  app.kubernetes.io/instance: "6f2f347f-a43b-43fb-9f72-2f6f47aa91be"
  app.kubernetes.io/part-of: "smart-city"
  app.kubernetes.io/managed-by: "edgelet"
  iofog.org/microservice-uid: "6f2f347f-a43b-43fb-9f72-2f6f47aa91be"
  iofog.org/node-uid: "node-123"
  iofog.org/scope: "managed"
  iofog.org/runtime-engine: "podman"
  iofog.org/role: "workload"
  iofog.org/system: "false"
  iofog.org/host-network: "false"
env:
  - IOFOG_MICROSERVICE_UID=6f2f347f-a43b-43fb-9f72-2f6f47aa91be
  - IOFOG_MICROSERVICE_NAME=video-analyzer
  - IOFOG_APPLICATION_NAME=smart-city
  - IOFOG_NODE_UID=node-123
  - IOFOG_SCOPE=managed
  - IOFOG_RUNTIME_ENGINE=podman
  - IOFOG_ROLE=workload
  - TZ=UTC
```

### iofog/containerd example

```yaml
labels:
  app.kubernetes.io/name: "video-analyzer"
  app.kubernetes.io/instance: "6f2f347f-a43b-43fb-9f72-2f6f47aa91be"
  app.kubernetes.io/part-of: "smart-city"
  app.kubernetes.io/managed-by: "edgelet"
  iofog.org/microservice-uid: "6f2f347f-a43b-43fb-9f72-2f6f47aa91be"
  iofog.org/node-uid: "node-123"
  iofog.org/scope: "managed"
  iofog.org/runtime-engine: "iofog"
  iofog.org/role: "workload"
  iofog.org/system: "false"
  iofog.org/host-network: "false"
  iofog.org/sandbox-id: "7d1a0d2f4b4f5f3f5f4f..."
env:
  - IOFOG_MICROSERVICE_UID=6f2f347f-a43b-43fb-9f72-2f6f47aa91be
  - IOFOG_MICROSERVICE_NAME=video-analyzer
  - IOFOG_APPLICATION_NAME=smart-city
  - IOFOG_NODE_UID=node-123
  - IOFOG_SCOPE=managed
  - IOFOG_RUNTIME_ENGINE=iofog
  - IOFOG_ROLE=workload
  - TZ=UTC
```

## Audit helpers (Go)

Package `internal/workloadmeta` exports `RemovedLegacyLabelKeys` and `RemovedLegacyEnvVars` as the single reference list for tests and repository audits. Runtime code must not branch on these slices.

## Notes for implementation phases

- Phase 0 changes only this documentation contract.
- Runtime implementation begins in Phase 1 and must follow this spec exactly.
- Phase 5 completes removal of all legacy identity metadata from runtime paths; engine-internal operational `iofog-*` keys (see above) remain only where explicitly documented.