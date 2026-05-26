# LocalAPI v1 Contract Freeze

This document freezes the LocalAPI v1 baseline used for implementation in this repository.

## Scope

- Canonical route namespace: `/v1/...`
- Baseline OpenAPI contract: `docs/localapi-v3-openapi.yaml`
- CLI must remain a thin transport client and should not implement daemon runtime logic.
- RuntimeClass surface is part of v3 contract (`/v1/deploy/runtimeclasses*`) with full+iofog gating.

## Locked Decisions

1. Dual transport:
   - Unix socket: `/run/edgelet/edgelet.sock`
   - HTTPS/WSS: `https://edgelet.default.svc.bridge.local`
2. JWT mode:
   - Unprovisioned: unsigned bootstrap JWT accepted across LocalAPI endpoints
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

- LocalAPI contract is v3-only in this repository.
- This freeze document is intentionally policy-focused; detailed endpoint shapes remain in OpenAPI YAML.
