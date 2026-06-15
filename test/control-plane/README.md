# ControlPlane integration tests

Lima-based IT for Edgelet-managed Controller (`kind: ControlPlane`).

Operator guide: [docs/edgelet/control-plane.md](../../docs/edgelet/control-plane.md)

## Fixture identity

`fixtures/controlplane-it.yaml` sets explicit **`metadata.namespace`** and **`metadata.name`** (default: `default` / `pot`). DNS assertions use these values — not a hardcoded microservice name `controller`.

| FQDN | Pattern |
|------|---------|
| 1 | `edgelet.controller.svc.bridge.local` |
| 2 | `controller.<namespace>.svc.bridge.local` |
| 3 | `<namespace>.<name>.svc.bridge.local` |

## Tests

| Case | Script | VM | Engine |
|------|--------|-----|--------|
| embedded apply | `t12-embedded.sh` | `iofog-test` | embedded |
| docker apply | `t12-docker.sh` | `edgelet-engine-lifecycle` | docker |
| controller status API | both | — | `curl :51121/api/v3/status` |
| lifecycle (unprovisioned) | both | — | `ms rm` allowed + reconcile; `controlplane delete` |
| DNS resolution | `t12-embedded.sh` only | — | nslookup 3 FQDNs from probe MS |

Deploy is **strict** (async apply): `cp_deploy` fails the suite on non-zero `edgelet deploy` exit. Async apply polls until terminal, then CLI checks `runtimeState=running`. Optional **`cp_wait_running`** sanity check after deploy.

## Run

```bash
./test/control-plane/run-all.sh
./test/control-plane/run-all.sh --case=embedded --skip-regression
./test/control-plane/run-all.sh --skip-build --skip-setup
```

Regression (default when `--case=all`): `test/workload-continuity/run-all.sh`, `test/embedded/run-all.sh` (`--skip-start`).

## Prerequisites

- macOS host with Lima
- Network pull for `ghcr.io/datasance/controller:3.8.0-beta.0` and router/nats images
- `go test` gates for packages are separate; this suite is end-to-end only
