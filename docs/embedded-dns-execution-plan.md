# Embedded DNS Execution Plan

## Phase 0 - Spec lock

### Deliverables

- Planning package approved.
- ADR draft approved.
- Normative D1-D9 behavior frozen.

### Exit gates

- No open policy questions.
- Pre-execution gate checklist fully green.

## Phase 1 - Contract and parity baseline

### Deliverables

- Cross-engine policy contract approved.
- Docker inconsistency remediation checklist approved.

### Exit gates

- Contract accepted by engine maintainers.
- Parity acceptance checks defined and testable.

## Phase 2 - Architecture validation

### Deliverables

- Component boundaries and interfaces validated.
- Security/observability requirements approved.

### Exit gates

- Startup/reconcile/failure behavior agreed.
- Required metrics and logs mapped to ops expectations.

## Phase 3 - Implementation slicing

## Slice order

1. Resolver server scaffold (UDP/TCP listeners + query dispatch).
2. Registry/index core + canonicalization utilities.
3. Lifecycle integration hooks (create/start/stop/remove/recover).
4. Reconcile loop + generation/version semantics.
5. Snapshot persistence and startup restore path.
6. Forwarding path with bounded controls.
7. Host-network publication and source-IP policy.
8. Compatibility alias policy gate.
9. Cross-engine contract enforcement and parity fixes.

### Exit gates

- Each slice includes unit tests.
- Slice dependencies and regression impact reviewed.

## Phase 3 Test matrix definition

### Unit tests

- Canonicalization normalization and alias generation.
- Reserved conflict tie-break determinism.
- Scope isolation and policy-denied behaviors.
- RCODE matrix behavior.
- Host-network IP source fallback.

### Integration tests

- CRI lifecycle event to DNS visibility transitions.
- Startup recovery from snapshot and runtime reconcile.
- Forwarding success and bounded failure behavior.
- Compatibility alias opt-in behavior.

### Churn/restart tests

- Burst create/start/stop/remove while resolving repeatedly.
- Restart agent under active workloads and verify convergence.
- Simulate missed events and verify reconcile repair.

### Airgapped tests

- Internal authoritative names fully functional with no upstream connectivity.
- Forwarding path degradation does not break internal resolution.

### Cross-engine parity tests

- Docker local/managed network and alias parity.
- Drift behavior consistency for `iofog` vs `iofog-local` expected networks.

## Phase 4 - Hardening gates

- Sustained resolver correctness under churn for defined interval.
- Bounded CPU/memory envelope under expected query rates.
- Forwarding degradation behavior validated under upstream outage.
- No unresolved high-severity issues in mandatory test matrix.

## Phase 4 - Runbook readiness

- DNS operational runbook published and reviewed.
- Incident triage steps validated against emitted telemetry.

## Phase 5 - Rollout plan

### Rollout stages

1. Internal test environment with feature flag.
2. Limited canary edge subset.
3. Expanded canary by topology class.
4. General availability rollout.

#### Stage entry/exit criteria

| Stage | Minimum soak window | Required pass criteria |
| --- | --- | --- |
| 1. Internal | 24h | PR6 embedded DNS script gates pass; no unresolved high-severity defects; `dnsHealth` remains `ready` or bounded `degraded` during induced forward outages. |
| 2. Limited canary | 48h | In-zone success SLO met; no sustained `SERVFAIL` breach; reconcile correction/error counters stable; rollback drill validated. |
| 3. Expanded canary | 72h | Same as stage 2 plus topology parity checks (managed/local scope behavior, restart convergence) and no persistent forwarding degradation. |
| 4. GA | 7d post-stage-3 | All SLO controls green for full soak window; no rollback trigger events in prior 7 days. |

### Rollback criteria

| Trigger | Threshold | Evaluation window | Action |
| --- | --- | --- | --- |
| In-zone success rate breach | `< 99.9%` | 10m rolling | Roll back one stage immediately. |
| `SERVFAIL` rate breach | `> 1%` of all DNS responses | 5m rolling | Enter rollback if not recovered within 10m. |
| Forwarding degraded saturation | `dnsForwardingDegraded=true` for `> 80%` of samples | 15m rolling | Keep internal-authoritative mode; roll back if forward path required for workload class. |
| Reconcile failure streak | `>= 5` consecutive failed runs | Immediate | Roll back one stage and open incident. |
| Snapshot failure rate | `>= 3` save/load failures | 15m rolling | Roll back one stage and disable snapshot-dependent promotion. |
| Cross-engine parity regression | Any deterministic parity test failure | Immediate | Block promotion and roll back canary stage. |

### SLO and acceptance controls

- Resolver readiness startup target: `<= 30s` to stable listener+status readiness after agent start.
- Query success target for in-zone names: `>= 99.9%` success (A/AAAA/ANY) per 10m rolling window.
- Mean/percentile forwarding latency thresholds:
  - mean `<= 50ms`
  - p95 `<= 200ms`
  - p99 `<= 500ms`
  during healthy-upstream windows.
- Guardrail acceptance:
  - rate-limit/drop counters can increase under synthetic load without causing daemon instability.
  - no uncontrolled CPU/memory growth during bounded flood scenarios.
