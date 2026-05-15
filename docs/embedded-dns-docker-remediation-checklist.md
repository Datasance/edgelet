# Docker Local-Network Parity Remediation Checklist

## Objective

Eliminate divergence between desired cross-engine policy and Docker path behavior for local workload networking and discovery aliases.

## Checklist

- [ ] Centralize network selection contract and remove hardcoded assumptions.
- [ ] Ensure local workloads map to `iofog-local` consistently in create path.
- [ ] Ensure alias endpoint attachment uses selected network, not fixed network key.
- [ ] Ensure drift checks validate expected network from shared policy.
- [ ] Ensure parity checks cover local and managed workload classes.
- [ ] Ensure host-network publication behavior is policy-aligned with full flavor.
- [ ] Ensure compatibility aliases are emitted only under explicit policy enablement.
- [ ] Add regression coverage for local-network behavior during updates/restarts.

## Parity acceptance criteria

- [ ] Local workload created under Docker resolves on expected local scope.
- [ ] Managed workload created under Docker resolves on managed scope.
- [ ] Drift logic does not flag valid local-network placements as mismatch.
- [ ] Alias set parity aligns with shared policy contract for equivalent workloads.
