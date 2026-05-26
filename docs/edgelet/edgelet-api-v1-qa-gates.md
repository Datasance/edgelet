# EdgeletAPI v1 QA Gates

This document defines execution gates for a strict v1-only EdgeletAPI surface.

## Gate 0: Contract Freeze

- `docs/edgelet-api-v1-openapi.yaml` exists and is treated as immutable baseline.
- Any route/schema/auth delta requires explicit confirmation before merge.

## Gate 1: Invariants

- `go test ./internal/processmanager ./internal/fieldagent ./internal/config`
- Verifies processmanager reconciliation guards, exec close semantics, and reload suppression.

## Gate 2: Transport/Auth

- `go test ./internal/auth ./internal/edgeletapi ./internal/serviceaccount`
- Verifies bootstrap/provisioned JWT policy and serviceaccount projection writes.

## Gate 3: Storage

- `go test ./internal/store`
- Verifies additive migration for:
  - `service_account_tokens`
  - `local_deployed_microservices`

## Gate 4: Runtime API Binding

- `go test ./internal/edgeletapi ./internal/runtimeapi`
- Validates EdgeletAPI handler wiring through facade adapters without runtime-engine behavior changes.

## Gate 5: CLI EdgeletAPI migration

Automated:

- `go test ./internal/cli/... ./cmd/edgelet`
- `make cli-docs-check` — generated docs under `docs/cli/` must match committed output
- CLI smoke tests (in `internal/cli/cmd/smoke_test.go`):
  - `-o json` stdout is valid JSON (`system status`, `ms ls`)
  - daemon down → exit **10**
  - legacy commands fail (`status`, `ms ps`, `deploy apply`, `config set`, …)
- Golden JSON fixtures match `docs/cli/output-schemas.md` (`internal/cli/output/schemas_test.go`)

Manual smoke (daemon running):

```bash
edgelet system status
edgelet system status -o json | jq .
edgelet ms ls -o json | jq '.items'
edgelet ms inspect <id>
edgelet deploy -f <manifest.yaml>
edgelet deploy -f <manifest.yaml> --dry-run
edgelet auth whoami -o json | jq .
```

Legacy must fail (exit non-zero):

```bash
edgelet status
edgelet ms ps
edgelet deploy apply -f <manifest.yaml>
edgelet config set foo bar
```

CLI reference: [docs/cli/README.md](cli/README.md)

## Gate 6: Embedded Validation Without ctr

- `test/embedded/vm-test.sh` must not install or invoke `ctr`.
- Runtime checks must execute via `edgelet` CLI only.

## Gate 7: RuntimeClass dual-shim VM coverage (aarch64)

- Embedded VM suite must validate RuntimeClass activation for both external shims:
  - Spin (`containerd-shim-spin`)
  - Edgelet (`containerd-shim-edgelet-wasm-v2`)
- Required checks:
  - shim download/install on VM
  - RuntimeClass validate/apply success (sync success OR async accepted + poll to terminal success)
  - controlled containerd restart convergence after apply
  - `availableRuntimes` reflects synthesized handlers
  - runtime-pinned workloads start for both RuntimeClasses
  - RuntimeClass delete guard rejects while runtime is in use (error includes blocking UUID)
  - after removing runtime-dependent workloads, RuntimeClass delete succeeds (sync success OR async accepted + poll to terminal success)
  - deleted RuntimeClass runtime entries are removed from effective runtime map after convergence

## Rollout Controls

- EdgeletAPI surface is v1-only; no route-level fallback to v2.
- Promote changes only after all gates pass in CI.

## Rollback Strategy

- Release-level rollback to prior v1-only release if critical regressions appear.
- Data safety: migrations are additive; no destructive schema changes in rollout phases.
