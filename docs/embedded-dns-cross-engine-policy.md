# Embedded DNS Cross-Engine Policy Contract

## Purpose

Define one shared policy contract for `iofog` (full flavor), Docker, and Podman engines to prevent divergent behavior across network selection, alias generation, and drift checks.

## Contract 1: Network selection

Given workload attributes, network selection **MUST** be deterministic and shared:

- `hostNetwork=true` -> host network semantics.
- `hostNetwork=false` -> canonical bridge network `iofog` for both local and managed workloads.

The same function **MUST** be used by:

- create path
- status/inspect normalization
- drift-comparison logic

## Contract 2: Alias generation

Alias generation **MUST** be centralized and deterministic:

- canonical workload aliases:
  - `appName.microserviceName`
  - `iofog_<microservice-uuid>`
- reserved aliases:
  - `router.default.svc.bridge.local`
  - `nats.default.svc.bridge.local`
  - `iofog.default.svc.bridge.local`
- optional compatibility aliases (policy controlled):
  - `host.docker.internal`
  - `host.container.internal`

Alias publication rules **MUST** be aligned with network scoping policy.

## Contract 3: Drift comparison

Drift checks **MUST** compare actual runtime state against desired state using the same policy functions used during create.

At minimum, drift evaluation **MUST** include:

- expected network target
- expected alias set
- host-network publication semantics
- record visibility eligibility (running/routable)

## Docker inconsistency remediation requirements

The following are mandatory:

- Do not diverge per-engine network selection behavior for non-host workloads.
- Keep local/managed as metadata-policy concept (labels/env), not distinct bridge identity.
- Drift checks **MUST** treat canonical `iofog` as expected non-host network across engines.

## Parity acceptance checks

1. Local workload created under Docker appears on canonical `iofog` with expected aliases.
2. Managed workload created under Docker appears on canonical `iofog` with expected aliases.
3. Drift check does not falsely flag correctly networked local workloads.
4. Full flavor and Docker/Podman generate equivalent alias sets for equivalent inputs.
5. Compatibility aliases are emitted only when explicit policy enables them.
