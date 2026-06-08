# ControlPlane (runtime)

The `internal/controlplane` package holds **pure conversion logic** for local Datasance Controller deployments: manifest → runtime microservice model, controller environment variables, port/volume mappings, and DNS FQDN helpers. Reconcile and persistence live in Process Manager, store, and runtimeapi.

**Code:** `internal/controlplane/`

**Operator guide:** [../control-plane.md](../control-plane.md)

## Purpose

- Build `models.Microservice` from validated `ControlPlaneManifest`
- Define default ports, volume names, container paths
- Merge capabilities and HTTPS cert bind mounts
- Supply DNS FQDN lists for bridge registration

Does **not** run HTTP or containers directly.

## Dependencies

| Depends on | Reason |
|------------|--------|
| `models` | Manifest and microservice types |

| Used by | Reason |
|---------|--------|
| `runtimeapi` | Deploy apply validates and persists manifest |
| `processmanager` | Launch/recreate CP container from SQLite row |
| `dnsresolver` | FQDN helpers for CP workload records |

## Manifest → microservice

`BuildMicroserviceFromControlPlane(doc, controllerUUID, image)`:

| Aspect | Default / rule |
|--------|----------------|
| UUID | Generated controller UUID (stable in SQLite) |
| `IsController` | `true`, `IsSystem` |
| Host API port | **51121** → `spec.controller.port` (default 51121) |
| Host viewer port | **80** → `spec.ecnViewerPort` (default 8008) |
| Volumes | `iofog-controller-db`, `iofog-controller-log` |
| HTTPS | Optional bind mount to `/etc/iofog/controller-cert/` |
| Registry | `RegistryID = 2` (local default) |

Constants in `runtime.go`: `HostAPIPort`, `HostViewerPort`, volume names, container paths.

## Environment

`BuildControllerEnv(doc, controllerUUID)` produces controller container env (Datasance Controller expectations). Called during microservice build.

## DNS

`ControlPlaneFQDNs(namespace, name)` returns bridge-local names consumed by dnsresolver when CP container is registered (see [dnsresolver.md](dnsresolver.md)).

## Persistence and reconcile

| Layer | Responsibility |
|-------|----------------|
| `store.system_control_plane` | Singleton deployment row + manifest YAML |
| `runtimeapi` | Async apply, validation, operation polling |
| `processmanager` | `reconcileControlPlane()` each monitor tick |
| `processmanager/controlplane_dns.go` | Upsert/remove DNS workload record |

At most **one** CP deployment per node (SQLite `id=1` check).

## External APIs

| EdgeletAPI | Role |
|------------|------|
| `POST /v1/deploy/controlplane:apply` | Start async apply |
| `GET /v1/deploy/controlplane:apply/{operationId}` | Poll status |
| `GET /v1/system/controlplane` | Deployment status |
| `DELETE /v1/system/controlplane` | Remove deployment + volumes |

## Failure modes

| Symptom | Cause |
|---------|-------|
| Reconcile recreates container | Row exists; external `docker rm` |
| `ms rm` rejected on CP UUID | Must use `controlplane delete` |
| Second apply 409 | Apply in progress |

## Code map

| File | Role |
|------|------|
| `runtime.go` | Microservice build, env, ports, volumes |
| `env.go` | Controller env construction |
| `runtime_test.go` | Conversion tests |

Related: [runtimeapi.md](runtimeapi.md), [processmanager.md](processmanager.md), [store.md](store.md), [../control-plane.md](../control-plane.md).
