# LocalAPI v3 QA Gates

This document defines execution gates for a strict v3-only LocalAPI surface.

## Gate 0: Contract Freeze

- `docs/localapi-v3-openapi.yaml` exists and is treated as immutable baseline.
- Any route/schema/auth delta requires explicit confirmation before merge.

## Gate 1: Invariants

- `go test ./internal/processmanager ./internal/fieldagent ./internal/config`
- Verifies processmanager reconciliation guards, exec close semantics, and reload suppression.

## Gate 2: Transport/Auth

- `go test ./internal/auth ./internal/localapi ./internal/serviceaccount`
- Verifies bootstrap/provisioned JWT policy and serviceaccount projection writes.

## Gate 3: Storage

- `go test ./internal/store`
- Verifies additive migration for:
  - `service_account_tokens`
  - `local_deployed_microservices`

## Gate 4: Runtime API Binding

- `go test ./internal/localapi ./internal/runtimeapi`
- Validates v3 handler wiring through facade adapters without runtime-engine behavior changes.

## Gate 5: CLI v3 Migration

- `go test ./internal/cli ./cmd/iofog-agent`
- Manual smoke:
  - `iofog-agent ms ps`
  - `iofog-agent ms inspect <id>`
  - `iofog-agent deploy -f <manifest.yaml>`
  - `iofog-agent auth whoami`

## Gate 6: Embedded Validation Without ctr

- `test/embedded/vm-test.sh` must not install or invoke `ctr`.
- Runtime checks must execute via `iofog-agent` CLI only.

## Rollout Controls

- LocalAPI surface is v3-only; no route-level fallback to v2.
- Promote changes only after all gates pass in CI.

## Rollback Strategy

- Release-level rollback to prior v3-only release if critical regressions appear.
- Data safety: migrations are additive; no destructive schema changes in rollout phases.
