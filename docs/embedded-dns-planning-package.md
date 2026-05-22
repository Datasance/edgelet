# Embedded DNS Planning Package for ioFog Full Flavor

## Section A - Executive Summary

ioFog full flavor needs Docker-like service discovery without introducing Kubernetes-style operational weight. The selected architecture is an embedded authoritative DNS subsystem inside `iofog-agentd`, designed for disconnected edge operation, deterministic behavior during runtime churn, and low resource footprint.

- Option A (embedded DNS) is selected as the primary architecture for full flavor.
- Runtime truth comes from embedded containerd + CRI lifecycle state.
- Discovery supports `appName.microserviceName`, `iofog_<microservice-uuid>`, and reserved core names.
- DNS keeps local/managed as metadata scopes while networking uses a single canonical bridge (`iofog`) for non-host workloads.
- Records become query-visible only when targets are running and routable.
- Reconcile loop + startup recovery ensure correctness after restart/crash.
- JSON snapshot improves warm-start convergence without introducing DB schema coupling.
- External DNS queries are forwarded by policy, with strict timeout/retry/circuit-break controls.
- Host-network workloads are discoverable, using `advertiseIP` fallback logic.
- Compatibility host aliases are supported as explicit opt-in (disabled by default).
- Security and observability requirements are mandatory gates before rollout.

## Section B - Decision Matrix (Weighted)

### Evaluation criteria and weights

| Criterion | Weight |
|---|---:|
| Containerd/CRI fit | 20% |
| Airgapped/offline operability | 20% |
| Runtime overhead | 15% |
| Correctness under churn/restart | 15% |
| Operability/observability | 10% |
| Security/isolation | 10% |
| Long-term maintainability | 10% |

### Option scoring

| Option | CRI fit | Offline | Overhead | Churn correctness | Operability | Security | Maintainability | Weighted outcome |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| A: embedded DNS in agent | 5 | 5 | 5 | 4 | 4 | 5 | 4 | Highest |
| B: CoreDNS per agent | 4 | 5 | 3 | 4 | 5 | 4 | 4 | Strong fallback |
| C: CNI-plugin-centric DNS chain | 2 | 4 | 4 | 2 | 2 | 3 | 2 | Not recommended |
| D: static `/etc/hosts` only | 3 | 5 | 5 | 1 | 1 | 3 | 3 | Insufficient for production |

### Decision

- **Primary**: Option A for full flavor.
- **Fallback**: Option B only where policy/compliance requires externalized DNS daemon behavior.

## Section C - Production Target Architecture (Option A)

### C1. Components and responsibilities

- **Authoritative DNS server**: UDP/TCP listeners bound to bridge-scoped addresses.
- **Registry/index**: in-memory canonical records keyed by network scope and normalized FQDN.
- **Lifecycle integrator**: idempotent updates from create/start/stop/remove/recover paths.
- **Reconcile loop**: periodic runtime truth repair for stale/missing records.
- **Snapshot/recovery**: atomic JSON snapshot write/read for warm restart.

### C2. Data model

- Identity keys: `networkScope + canonicalName + rrType`.
- Payload: `targetIP`, `msUUID`, `sourceKind`, `state`, `generation`, timestamps.
- Canonicalization: lowercase FQDN storage, normalized trailing-dot semantics.

### C3. Query path

- Bind address identifies network scope.
- Authoritative names return scoped answers only.
- In-scope unknown names return authoritative negative response.
- Out-of-zone names are forwarded by configured policy.
- Forwarding is bounded by timeout, retry, and circuit-break rules.

### C4. Runtime integration points

- Create: register pending record.
- Start: publish as active.
- Stop/remove: withdraw immediately.
- Recover: rebuild from runtime labels/status at startup.
- Reconcile: enforce eventual correctness under missed events.

### C5. Performance envelope assumptions

- O(1)-style lookups for steady-state queries.
- Linear memory growth to active record count with bounded metadata.
- DNS query CPU overhead significantly below container lifecycle operations.

### C6. Failure model and self-healing

- Snapshot corruption: ignore snapshot, rebuild from runtime.
- Runtime API transient failure: preserve last known good, retry reconcile.
- Upstream DNS outage: serve internal authoritative names, degrade forwarding path.

## Section D - Normative Behavior Specification (RFC 2119)

### D1. Name canonicalization rules

- Resolver **MUST** lowercase incoming QNAME before lookup.
- Resolver **MUST** normalize trailing-dot and non-trailing-dot variants.
- Canonical storage **MUST** use normalized FQDN.

### D2. Zone and network scoping

- Resolver **MUST** enforce separate metadata scopes for local and managed workloads.
- Resolver **MUST NOT** return cross-scope answers by default.
- Any cross-scope behavior **MUST** require explicit opt-in policy.

### D3. Record publication rules

- Records **MUST NOT** be query-visible before target is running/routable.
- Stop/remove events **MUST** withdraw active records immediately.
- Startup readiness **MUST** depend on post-recovery runtime reconciliation.

### D4. Reserved-name conflict resolution

- Reserved names:
  - `router.default.svc.bridge.local`
  - `nats.default.svc.bridge.local`
  - `iofog.default.svc.bridge.local`
- If multiple eligible `router`/`nats` targets exist, selection **MUST** be deterministic:
  - newest running target wins;
  - lexical `msUUID` tie-break.
- Conflict events **MUST** emit warning logs and conflict metrics.

### D5. Response code matrix

| Scenario | RCODE | Answer section | TTL behavior | Required telemetry |
|---|---|---|---|---|
| Known name + healthy target | `NOERROR` | active A/AAAA records | positive TTL | success counter |
| Known name + inactive target | `NOERROR` | empty answer (NODATA) | short negative/empty TTL | inactive-target counter |
| Unknown name in authoritative zone | `NXDOMAIN` | none | negative TTL | nxdomain counter |
| Name exists but denied by scope policy | `NXDOMAIN` | none | negative TTL | policy-denied counter |
| Internal transient resolver/runtime error | `SERVFAIL` | none | none/zero | servfail counter + rate-limited error logs |
| External name with forwarding enabled | upstream-derived | upstream-derived | upstream-derived (optionally capped) | forward query, latency, upstream error counters |

### D6. TTL and negative caching policy

- Positive TTL **SHOULD** default to 5-15 seconds.
- Negative TTL **SHOULD** default to 1-3 seconds.
- TTLs **MUST** be centrally configurable with safe defaults.

### D7. Reconcile and recovery semantics

- Reconcile loop **MUST** run periodically (recommended 5-15 seconds).
- Startup sequence **MUST**: load snapshot -> validate snapshot -> runtime reconcile -> set ready.
- Record generation/version **MUST** be monotonic per identity key.

### D8. Host-network and offline/airgapped rules

- Host-network workloads **MUST** be publishable for discovery (policy selected).
- Host-network DNS target IP **MUST** use `advertiseIP` when set.
- If `advertiseIP` is unset, resolver **MUST** fall back to detected primary agent IP.
- Internal authoritative resolution **MUST** remain available offline and without controller connectivity.

### D9. Compatibility alias rules

- `iofog.default.svc.bridge.local` **MUST** remain canonical host endpoint name.
- `host.docker.internal` and `host.container.internal` **MAY** be provided as compatibility aliases.
- Compatibility aliases **MUST** be disabled by default and enabled only by explicit compatibility policy.
- When enabled, compatibility aliases **MUST** resolve to same target as canonical host endpoint.

## Section E - Security and Isolation Requirements

- DNS sockets **MUST** bind only to intended bridge-scoped addresses.
- Query parsing **MUST** enforce input validation limits (label lengths, packet size, malformed payload handling).
- Forwarding path **MUST** enforce bounded timeout/retry/circuit-break behavior.
- Resolver logs **MUST** be rate-limited for error/flood scenarios.
- Registry mutation APIs **MUST** be agent-internal only.

## Section F - Operability and Observability Baseline

- Readiness **MUST** require initial runtime reconcile convergence.
- Liveness **MUST** indicate resolver process health independent of transient upstream DNS failures.
- Metrics **MUST** include:
  - total queries by outcome (`success`, `nxdomain`, `servfail`, policy denied),
  - forwarding query/latency/upstream errors,
  - active records by network scope,
  - reconcile runs/corrections/failures,
  - reserved-name conflict count,
  - snapshot load/save success/failure.
- Logs **MUST** include:
  - startup recovery summary,
  - reconcile correction summaries,
  - reserved-name conflict events,
  - forwarding degradation events.
- Alerting **SHOULD** include:
  - sustained `SERVFAIL` rate,
  - reconcile failure streak,
  - prolonged not-ready condition,
  - persistent reserved-name conflict.

## Section G - Cross-Engine Policy Alignment Baseline

- A shared policy contract **MUST** define:
  - network selection rules,
  - alias generation rules,
  - drift-comparison semantics.
- Docker local-network inconsistency remediation:
  - non-host workloads **MUST** validate expected target network (`iofog`) by shared policy, not hardcoded assumptions;
  - alias endpoint assignment **MUST** match selected network;
  - drift checks **MUST** validate against same contract used at create time.
- Compatibility mode **SHOULD** support controlled staged adoption.

## Section J - Risk Register (Top 10)

| Risk | Impact | Likelihood | Mitigation | Owner | Trigger | Fallback |
|---|---|---|---|---|---|---|
| Stale records under churn | High | Medium | event + reconcile dual-path, short TTL | Runtime networking lead | stale correction spikes | tighten reconcile interval |
| Router/NATS multi-candidate conflicts | High | Medium | deterministic tie-break + alerts | Platform architect | conflict metric persists | policy override/manual pin |
| Cross-scope leakage | High | Low-Medium | strict scope checks + policy-denied NXDOMAIN | Security lead | anomalous denied lookups | enforce strict isolation mode |
| Cold-start inconsistency | High | Medium | readiness after reconcile only | Runtime lead | startup not-ready timeout | fail closed until reconcile succeeds |
| Snapshot corruption | Medium | Medium | atomic write + checksum/version | Runtime lead | snapshot parse failure | runtime-only rebuild |
| Edge CPU pressure under query bursts | Medium | Medium | bounded parsing, efficient indexes | Performance engineer | query-correlated CPU spikes | TTL tuning + load shaping |
| Observability gaps | High | Medium | mandatory telemetry baseline | SRE lead | incidents without diagnostics | rollout hold |
| Engine behavior divergence | High | Medium | shared contract + parity tests | Multi-engine maintainer | engine-specific regressions | compatibility guardrails |
| Host-network publication misreachability | Medium-High | Medium | explicit source-IP policy + tests | Runtime networking lead | host-network reachability incidents | policy-based publication controls |
| Upstream forwarding instability | Medium | Medium | bounded forwarding + circuit-break | Ops engineering | forward latency/error spikes | internal-authoritative-only degradation |
