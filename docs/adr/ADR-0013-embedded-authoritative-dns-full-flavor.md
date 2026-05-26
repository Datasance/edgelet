# ADR-0013: Embedded Authoritative DNS for Full Flavor

- Status: Proposed
- Date: 2026-05-15
- Owners: Runtime/Networking maintainers
- Scope: `containerEngine=iofog` (full flavor), with cross-engine policy alignment requirements

## Context

ioFog full flavor currently relies on static `/etc/hosts` and host `resolv.conf` mounting for bridge workloads. This does not provide robust dynamic service-name resolution under churn and restart-heavy edge conditions. The platform requires low-footprint, offline-capable, deterministic discovery behavior across geographically distributed and frequently disconnected edge nodes.

## Decision

Adopt an embedded authoritative DNS subsystem inside `edgelet daemon` (Option A) for full flavor workloads.

## Decision drivers

- Containerd/CRI lifecycle fit
- Airgapped/offline operability
- Runtime overhead constraints
- Correctness under churn/restart
- Operability and observability
- Security/isolation requirements
- Long-term maintainability

## Considered options

1. Embedded DNS in agent (selected)
2. CoreDNS per agent (fallback profile)
3. CNI-plugin-centric DNS chain (not selected)
4. Static `/etc/hosts` only (insufficient)

## Consequences

### Positive

- Best fit with embedded runtime ownership model.
- No additional daemon/process in baseline path.
- Better deterministic behavior in disconnected edge deployments.

### Tradeoffs

- Agent owns DNS lifecycle logic and associated test burden.
- Requires robust reconciliation and observability to avoid silent drift.

## Normative commitments summary

- Reserved conflict tie-break: newest running target, lexical UUID tie-break.
- Known internal name with inactive target: `NOERROR` with empty answer (NODATA).
- Host-network workloads are discoverable.
- Host-network advertised IP source: `advertiseIP`, fallback to detected primary IP.
- External names are forwarded by policy with bounded forwarding controls.
- Compatibility host aliases are opt-in and disabled by default.

## Rollout strategy

- Feature-flagged staged rollout.
- Canary-first progression with explicit health, correctness, and error-budget gates.
- Controlled compatibility mode for cross-engine parity migration.

## Verification and acceptance criteria

- Functional DNS behavior parity with normative spec.
- Churn/restart convergence validated under stress.
- Airgapped behavior validated without controller connectivity.
- Cross-engine network/alias/drift parity tests pass.
- Observability and runbook readiness complete before broad rollout.

## Rollback strategy

- Disable feature flag and revert to previous resolver path.
- Keep runtime stable and continue serving baseline compatibility behavior.
- Preserve telemetry for postmortem and retry planning.

## Open questions

None pending at ADR draft time. Pre-implementation policy decisions are locked in the planning package.
