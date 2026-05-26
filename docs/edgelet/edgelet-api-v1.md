# EdgeletAPI v1

The **EdgeletAPI** is the on-device operator API exposed by the Edgelet daemon. The `edgelet` CLI is a thin client over this API.

> **Not the Controller API.** Remote Pot/Controller REST remains at `/api/v3/...` on the controller URL. EdgeletAPI is localhost-only administration (`/v1/...`).

## Base URL and transport

| Transport | URL |
|-----------|-----|
| HTTPS (default) | `https://127.0.0.1:54321` |
| Unix socket | `http+unix:///run/edgelet/edgelet.sock` |
| WebSocket | `wss://127.0.0.1:54321` (control/message channels) |

TLS server name: `edgelet.default.svc.bridge.local`.

## Authentication

Send the contents of `/etc/edgelet/edgelet-api` as a bearer token:

```bash
TOKEN=$(sudo cat /etc/edgelet/edgelet-api)
curl -sk --cacert /etc/edgelet/edgeletapi-ca.crt \
  -H "Authorization: Bearer ${TOKEN}" \
  https://127.0.0.1:54321/v1/system/status
```

JWT claims for CLI admin tokens:

| Claim | Value |
|-------|--------|
| `tokenUse` | `edgeletapi` |
| `aud` | `edgelet://edgeletapi/v1` |

**Bootstrap mode** (unprovisioned): unsigned bootstrap JWT accepted. **Provisioned**: signed Ed25519 required globally.

## Route groups

| Prefix | Purpose |
|--------|---------|
| `/v1/system/*` | Status, info, version, reload, stop, prune, config, GPS, provision |
| `/v1/ms/*` | Microservice list, inspect, lifecycle, logs, exec |
| `/v1/deploy/*` | Manifest apply/validate (Microservice, Registry, RuntimeClass) |
| `/v1/auth/*` | whoami, token list, revoke |
| `/v1/images/*` | Image list, pull, load, prune |

Unauthenticated probes: `/health/live`, `/health/ready`, `/metrics`.

## Response envelope

Successful responses wrap data:

```json
{
  "success": true,
  "data": { }
}
```

Errors:

```json
{
  "success": false,
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "..."
  }
}
```

Stable codes: [edgelet-api-v1-error-codes.md](edgelet-api-v1-error-codes.md).

## Deploy apply semantics

Manifest apply uses `multipart/form-data`:

| Field | Required | Description |
|-------|----------|-------------|
| `manifest` | yes | YAML manifest (`apiVersion: edgelet.iofog.org/v1`) |
| `dryRun` | no | Validate only (`200`, no persistence) |
| `async` | no | Return `202` with `operationId` for background apply |

Poll async operations via `GET /v1/deploy/{kind}:apply/{operationId}`.

## RuntimeClass (full + edgelet only)

RuntimeClass endpoints under `/v1/deploy/runtimeclasses*` require **full** build flavor and `containerEngine=edgelet`. Other modes return `400 INVALID_ARGUMENT`.

See OpenAPI for full request/response shapes and RBAC mapping: [edgelet-api-v1-rbac-resources.md](edgelet-api-v1-rbac-resources.md).

## Contract references

| Document | Description |
|----------|-------------|
| [edgelet-api-v1-openapi.yaml](edgelet-api-v1-openapi.yaml) | OpenAPI 3.1 baseline |
| [edgelet-api-v1-contract-freeze.md](edgelet-api-v1-contract-freeze.md) | Change-control policy |
| [edgelet-api-v1-qa-gates.md](edgelet-api-v1-qa-gates.md) | CI/QA gates |
| [edgelet-api-v1-error-codes.md](edgelet-api-v1-error-codes.md) | Error taxonomy |

CLI mapping: [../cli/README.md](../cli/README.md).
