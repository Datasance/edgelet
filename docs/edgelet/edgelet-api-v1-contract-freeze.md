# EdgeletAPI v1 Contract Freeze

This document freezes the EdgeletAPI v1 baseline used for implementation in this repository.

## Scope

- Canonical route namespace: `/v1/...`
- Baseline OpenAPI contract: `docs/edgelet/edgelet-api-v1-openapi.yaml`
- CLI must remain a thin transport client and should not implement daemon runtime logic.
- RuntimeClass surface is part of the v1 contract (`/v1/deploy/runtimeclasses*`) with full+edgelet gating.

## Locked Decisions

1. Dual transport:
   - Unix socket: `/run/edgelet/edgelet.sock`
   - HTTPS/WSS: `https://edgelet.default.svc.bridge.local`
2. JWT mode:
   - Unprovisioned: unsigned bootstrap JWT accepted across EdgeletAPI endpoints
   - Provisioned: unsigned JWT rejected globally; signed Ed25519 required
   - Deprovisioned: revert to bootstrap mode
3. API-group claims mapping:
   - `edgelet.iofog.org/v1` and `edgelet.datasance.com/v1` rules under `edgelet.iofog.org`
   - Other API groups passed raw under their group keys

## Change Control Rule

Implementation must pause and request explicit user confirmation before any of the following:

- endpoint path or method change from frozen OpenAPI baseline
- auth requirement changes
- request/response schema changes
- behavior changes in bootstrap/provisioned token acceptance
- changes to canonical namespace away from `/v1/...`

## Notes

- EdgeletAPI contract is v1-only in this repository.
- This freeze document is intentionally policy-focused; detailed endpoint shapes remain in OpenAPI YAML.
