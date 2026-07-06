# Edgelet documentation

Operator and developer documentation for the Edgelet edge agent.

## Architecture and deployment

| Document | Description |
|----------|-------------|
| [architecture.md](architecture.md) | Module layout, data flows, EdgeletAPI vs Controller API |
| [modules/README.md](modules/README.md) | Runtime module deep dives (all tiers) |
| [installation.md](installation.md) | install.sh, OTA, upgrade/rollback, controller readiness |
| [deployment.md](deployment.md) | Production topology, engines, systemd, provisioning |
| [persistence.md](persistence.md) | SQLite backup/restore, wipe-only upgrade, secrets threat model |
| [troubleshooting.md](troubleshooting.md) | Daemon, containerd, auth, CLI connectivity |
| [logging.md](logging.md) | Structured events, log levels, journald queries |

## Runtime and workloads

| Document | Description |
|----------|-------------|
| [container-engine.md](container-engine.md) | `edgelet` / `docker` / `podman` engines, CNI, RuntimeClass |
| [dns.md](dns.md) | Bridge DNS, embedded resolver, docker/podman aliases and ExtraHosts |
| [workload-metadata.md](workload-metadata.md) | Container labels and `EDGELET_*` env contract |
| [workload-continuity.md](workload-continuity.md) | Reconcile behavior across restarts and engine changes |
| [volumes.md](volumes.md) | Volume types, delete vs prune behavior, disk layout |
| [edgeguard.md](edgeguard.md) | Hardware attestation (`edgeGuardFrequency`) |
| [control-plane.md](control-plane.md) | Local Datasance Controller deployment |
| [exec-sessions.md](exec-sessions.md) | Multi-session exec (local CLI and controller-initiated) |
| [manifest-reference.md](manifest-reference.md) | Deploy YAML (`Microservice`, `Registry`, `RuntimeClass`, `ControlPlane`) |
| [examples/](examples/) | Reference manifest YAML samples |

## EdgeletAPI

| Document | Description |
|----------|-------------|
| [edgelet-api-v1.md](edgelet-api-v1.md) | Operator guide — transport, auth, errors, route behavior |
| [edgelet-api-v1-openapi.yaml](edgelet-api-v1-openapi.yaml) | OpenAPI 3.1 contract |
| [edgelet-api-v1-rbac-resources.md](edgelet-api-v1-rbac-resources.md) | RBAC resource/verb mapping |

## Migration

| Document | Description |
|----------|-------------|
| [migration-from-iofog-agent-cli.md](migration-from-iofog-agent-cli.md) | Legacy CLI → `edgelet` command mapping |

## CLI reference

| Resource | Path |
|----------|------|
| CLI overview | [../cli/README.md](../cli/README.md) |
| JSON/YAML output shapes | [../cli/output-schemas.md](../cli/output-schemas.md) |
| Generated per-command pages | [../cli/generated/](../cli/generated/) |

## Legal

| Document | Path |
|----------|------|
| License (EPL-2.0) | [../../LICENSE](../../LICENSE) |
| Copyright notice | [../../NOTICE](../../NOTICE) |
| Maintainers | [../../MAINTAINERS](../../MAINTAINERS) |
