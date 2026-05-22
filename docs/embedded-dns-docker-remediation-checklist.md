# Docker Single-Bridge Parity Remediation Checklist

## Objective

Eliminate divergence between desired cross-engine policy and Docker path behavior for single-bridge networking and discovery aliases.

## Checklist

- [ ] Centralize network selection contract and remove hardcoded assumptions.
- [ ] Ensure local and managed non-host workloads map to canonical `iofog` consistently in create path.
- [ ] Ensure alias endpoint attachment uses selected network, not fixed network key.
- [ ] Ensure drift checks validate expected network from shared policy.
- [ ] Ensure parity checks cover local and managed workload classes.
- [ ] Ensure host-network publication behavior is policy-aligned with full flavor.
- [ ] Ensure compatibility aliases are emitted only under explicit policy enablement.
- [ ] Add regression coverage for metadata-scope behavior during updates/restarts.

## Parity acceptance criteria

- [ ] Local workload created under Docker resolves on expected metadata scope.
- [ ] Managed workload created under Docker resolves on managed metadata scope.
- [ ] Drift logic does not flag valid canonical-network placements as mismatch.
- [ ] Alias set parity aligns with shared policy contract for equivalent workloads.
