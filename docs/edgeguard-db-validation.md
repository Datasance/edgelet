# EdgeGuard DB Validation Checklist

Use this checklist after deploying the DB-backed Edge Guard + private key changes.

## Preconditions
- Agent binary includes migration `003_edgeguard_and_credentials.sql`.
- SQLite database path is available and writable.
- Controller is reachable for provisioning.

## Provisioning and Key Persistence
- Provision agent using local API or CLI.
- Verify `agent_credentials` contains one row (`id=1`) with non-empty `private_key_b64`.
- Verify config YAML does not contain a non-empty `privateKey` value.
- Verify JWT-authenticated requests to controller succeed.

## Edge Guard Enable / Disable
- Set `edgeGuardFrequency` to a positive value while provisioned.
- Verify `edgeguard_signature` row is created (`id=1`) after first attestation.
- Set `edgeGuardFrequency` back to `0`.
- Verify `edgeguard_signature` row is deleted.

## Unprovisioned Invariant
- Ensure agent is unprovisioned (`iofogUuid` empty).
- Attempt to set `edgeGuardFrequency > 0`.
- Verify runtime frequency is normalized to `0` and Edge Guard does not start.

## Reprovision Behavior
- Reprovision with a new provisioning key.
- Verify `agent_credentials.private_key_b64` is updated.
- Verify `edgeguard_signature` baseline is cleared and re-created on next Edge Guard cycle.

## Deprovision Behavior
- Deprovision agent.
- Verify `edgeGuardFrequency` becomes `0`.
- Verify both `agent_credentials` and `edgeguard_signature` tables are empty.
- Verify in-memory auth is blocked (JWT generation returns not provisioned / blocked).

## Legacy File Check
- Verify no new `/etc/iofog-agent/agent-*.jwt` files are created.

## DB Failure Degradation
- Simulate SQLite unavailability.
- Verify process remains alive and local API remains reachable.
- Verify Edge Guard and private-key dependent auth/provision/reconcile paths are blocked.
