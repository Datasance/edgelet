# Plan 11 — Workload continuity integration tests

> **Spec:** [.cursor/edgelet/docs/11-workload-continuity.md](../../.cursor/edgelet/docs/11-workload-continuity.md) §Phase 11-5  
> **Skill:** `@edgelet-plan-11-workload-continuity`

## Test matrix

| ID | Script | When | Pass criteria |
|----|--------|------|---------------|
| **T11-A** | `docker-restart.sh` | After 11-1 | `restart edgelet` → same Docker container IDs; MS running |
| **T11-B** | (regression) | After 11-1 | `test/engine-lifecycle/run-all.sh` still green |
| **T11-C** | `embedded-restart.sh` | After 11-4 | restart **edgelet** only → CRI pods survive |
| **T11-D** | `embedded-runtime-restart.sh` | After 11-4 | restart **edgelet-containerd** → MS down then reconciled |
| **T11-E** | doc only | Pre-11-4 | Monolithic embedded restart still drains MS |

## Runner (to implement in Plan 11-5)

```bash
./test/workload-continuity/run-all.sh
./test/workload-continuity/run-all.sh --case=docker-restart
./test/workload-continuity/run-all.sh --case=embedded-restart
```

## Prerequisites

- T11-A: Lima or Linux VM with Docker + edgelet installed
- T11-C/D: Lima embedded IT profile (same family as `test/embedded/`)

## Status

**Not implemented** — README scaffold only (docs pass).
