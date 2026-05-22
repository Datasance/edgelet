# Embedded DNS Operations Runbook

## Scope

Operational procedures for full-flavor embedded DNS service discovery.

## Health checks

1. Verify resolver readiness has been reached post-startup recovery.
2. Verify reconcile loop is active and not in persistent failure.
3. Verify active record counts by metadata scope (`iofog`, `iofog-local`) are plausible while listener remains on canonical `iofog` bridge gateway.

## Key telemetry to inspect

- Query outcomes:
  - success
  - nxdomain
  - servfail
  - policy denied
- Forwarding:
  - upstream query rate
  - forwarding latency
  - upstream error rate
- Reconcile:
  - run count
  - correction count
  - failure streak
- Reserved conflict:
  - router/nats conflict counters
- Snapshot:
  - load/save failures

### SLO mapping

- In-zone success / `SERVFAIL` rollback triggers:
  - `iofog_dns_success_total`
  - `iofog_dns_servfail_total`
- Forwarding degradation triggers:
  - `iofog_dns_forwarding_degraded`
  - `iofog_dns_forward_err_total`
  - `iofog_dns_forward_upstreams_healthy`
- Reconcile stability triggers:
  - `iofog_dns_reconcile_runs_total`
  - `iofog_dns_reconcile_error_total`
- Snapshot durability triggers:
  - snapshot load/save failure logs and counters (when exported)

## Incident triage

### Symptom: known service names intermittently fail

1. Confirm workload running/routable state.
2. Confirm in-scope query path (correct network scope).
3. Check inactive-target counters and reconcile corrections.
4. Inspect recent lifecycle and reconcile logs.

### Symptom: excessive `NXDOMAIN`

1. Validate query names are canonicalized as expected.
2. Verify scope policy (cross-scope lookups should not resolve by default).
3. Check compatibility alias policy state.

### Symptom: excessive `SERVFAIL`

1. Distinguish internal resolver/runtime errors from forwarding upstream failures.
2. Check runtime client health and reconcile failures.
3. If upstream failures dominate, switch to internal-authoritative-only degradation mode if required.

### Symptom: wrong reserved target (router/nats)

1. Check conflict counters and warning logs.
2. Verify deterministic tie-break selection (newest running, UUID tie-break).
3. Confirm target role labels are correct and singular where expected.

## Safe fallback actions

- Enforce internal-authoritative-only mode during upstream instability.
- Tighten reconcile interval temporarily for stale-record incidents.
- Disable compatibility aliases if alias collisions or confusion is observed.
- Use feature flag rollback criteria defined in rollout plan when SLO thresholds are exceeded.

## Rollback procedure (PR6)

1. Confirm rollback trigger against execution-plan thresholds (error rate, `SERVFAIL`, reconcile failures, forwarding degradation persistence).
2. Freeze rollout progression for current stage and notify incident channel.
3. Roll back one stage:
   - disable embedded DNS feature flag for impacted cohort (or revert to prior release bundle if flag not available),
   - preserve logs/metrics snapshots for postmortem.
4. Verify rollback success within 15 minutes:
   - `iofog-agent system status` shows expected non-regressed DNS health state,
   - in-container probe for internal authoritative names succeeds,
   - `SERVFAIL`/error rates return below threshold windows.
5. Keep canary frozen until root cause and forward fix are validated through PR6 script gates.
