# Edgelet documentation

Operator and developer documentation for the Edgelet edge agent.

| Document | Description |
|----------|-------------|
| [architecture.md](architecture.md) | Module layout, data flows, EdgeletAPI vs Controller API |
| [deployment.md](deployment.md) | Install, systemd, full/lite flavors, provisioning |
| [persistence.md](persistence.md) | SQLite backup/restore, wipe-only upgrade, secrets threat model |
| [troubleshooting.md](troubleshooting.md) | Daemon, containerd, auth, CLI connectivity |
| [logging.md](logging.md) | Structured events, log levels, journald queries |
| [container-engine.md](container-engine.md) | `edgelet` / `docker` / `podman` engines, CNI, RuntimeClass |
| [edgelet-api-v1.md](edgelet-api-v1.md) | EdgeletAPI operator guide (HTTPS `:54321`, `/v1/`) |
| [edgelet-api-v1-openapi.yaml](edgelet-api-v1-openapi.yaml) | OpenAPI 3.1 contract |
| [edgelet-api-v1-contract-freeze.md](edgelet-api-v1-contract-freeze.md) | Frozen contract policy |
| [edgelet-api-v1-error-codes.md](edgelet-api-v1-error-codes.md) | Stable error taxonomy |
| [edgelet-api-v1-qa-gates.md](edgelet-api-v1-qa-gates.md) | QA gates for API surface |
| [edgelet-api-v1-rbac-resources.md](edgelet-api-v1-rbac-resources.md) | RBAC resource/verb mapping |
| [migration-from-iofog-agent-cli.md](migration-from-iofog-agent-cli.md) | Legacy CLI → `edgelet` command mapping |

## CLI reference

| Resource | Path |
|----------|------|
| CLI overview | [../cli/README.md](../cli/README.md) |
| JSON/YAML output shapes | [../cli/output-schemas.md](../cli/output-schemas.md) |
| Generated per-command pages | [../cli/generated/](../cli/generated/) |

## Related docs (repo root)

| Document | Description |
|----------|-------------|
| [../embedded-dns-runbook.md](../embedded-dns-runbook.md) | Embedded authoritative DNS (full flavor) |
| [../FEATURE-PARITY.md](../FEATURE-PARITY.md) | Java → Go feature parity checklist |
| [../metadata-labelspec-envspec.md](../metadata-labelspec-envspec.md) | Container metadata contract |
