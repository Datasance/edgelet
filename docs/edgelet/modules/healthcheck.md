# Healthcheck Runner

The healthcheck runner executes **exec-based container health checks** for the **edgelet engine only**. Docker and Podman rely on native engine healthcheck support and do not use this runner.

**Code:** `internal/healthcheck/`

## Purpose

- Poll running containers on an interval
- Run healthcheck commands via engine exec (`HealthcheckEngine` interface)
- Track consecutive failures against microservice `Retries` policy
- Update Process Manager / status when health state changes

## Dependencies

| Depends on | Reason |
|------------|--------|
| `processmanager` / engine | Container list and exec |
| `fieldagent` | `MicroserviceProvider` — healthcheck JSON on microservice model |
| `statusreporter` | Publish health transitions |
| `config` | `healthcheckIntervalSeconds` |

| Used by | Reason |
|---------|--------|
| `supervisor` | Started only when `containerEngine=edgelet` and engine implements `HealthcheckEngine` |

## Lifecycle

### Start

`NewRunner(engine, healthcheckEngine, fieldAgent).Start(ctx)`:

- **No-op** if `healthcheckEngine == nil` (docker/podman)
- Default interval: 30s if `healthcheckIntervalSeconds` ≤ 0

Started in Supervisor after Process Manager wiring:

```go
if cfg.ContainerEngine == constants.EngineEdgelet {
    s.healthcheckRunner = healthcheck.NewRunner(eng, hcEng, s.fieldAgent)
    s.healthcheckRunner.Start(s.ctx)
}
```

### Stop

Cancel context; wait for runner goroutine.

## Check flow

1. List running containers from engine
2. Resolve microservice by UUID via Field Agent
3. Parse `healthcheck` field from microservice model (JSON)
4. `ExecWithExitCode` with timeout
5. Increment/decrement consecutive failure counter
6. After retry threshold, mark unhealthy in status reporter

Uses `workloadmeta` helpers for label/env context where needed.

## Configuration

| Key | Default | Effect |
|-----|---------|--------|
| `healthcheckIntervalSeconds` | 30 | Poll interval |

Per-microservice healthcheck spec comes from Controller or manifest (`healthcheck` block).

## External APIs

No HTTP surface. Health reflected in:

- Process Manager microservice status
- Controller status POST (via Field Agent aggregation)

## Observability

- Log module: `"HealthcheckRunner"`
- Debug logs on exec failures and state transitions

## Failure modes

| Symptom | Typical cause |
|---------|----------------|
| Runner not started | docker/podman engine selected |
| Checks skipped | Missing healthcheck on microservice |
| Flapping unhealthy | Short timeout vs slow startup |

## Code map

| File | Role |
|------|------|
| `runner.go` | Main loop, exec checks, failure tracking |
| `runner_test.go` | Unit tests |

Related: [engines.md](engines.md), [processmanager.md](processmanager.md), [fieldagent.md](fieldagent.md).
