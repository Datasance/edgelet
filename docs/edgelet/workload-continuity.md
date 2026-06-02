# Edgelet workload continuity (operator guide)

> **Status:** Placeholder — filled during Plan 11 implementation.  
> **Spec:** [.cursor/edgelet/docs/11-workload-continuity.md](../../.cursor/edgelet/docs/11-workload-continuity.md)  
> **Contract:** [.cursor/edgelet/WORKLOAD-CONTINUITY.md](../../.cursor/edgelet/WORKLOAD-CONTINUITY.md)

## Principle (locked)

Restarting **`edgelet`** (control plane) must **not** stop microservice containers unless you restart the **runtime** unit or perform a **cold engine change** (Plan 9A).

## Topics (Plan 11)

| Topic | Detail |
|-------|--------|
| docker/podman | Default `shutdownPolicy=leave-running` |
| embedded | `edgelet-containerd.service` owns containerd + MS; `edgelet.service` reconciles only |
| Status | Brief MS UNKNOWN OK; `runtime.agentPhase=restarting` |
| OTA | Agent-only vs runtime bundle → which unit to restart |
| Engine change | Unchanged Plan 9A cold path (quiesce, cleanup, recreate) |

## Before runtime split (Plan 11-4)

Monolithic `edgelet` restart on **embedded** engine still drains MS — expect brief outage until Plan 11-4 ships.
