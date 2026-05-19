# ioFog Agent — Runtime Logging

Structured lifecycle logging for ProcessManager and container engines. Events are emitted via `internal/runtimeops` and written as **top-level JSON fields** in `iofog-agent` logs (logrus JSON formatter).

## Field schema

| Field | Description |
|-------|-------------|
| `timestamp` | Log time (logrus) |
| `level` | debug / info / warn / error |
| `module` | Logger module name (e.g. `Container Manager`) |
| `message` | Short human-readable summary |
| `event` | Stable event name (see catalog below) |
| `operationId` | Correlation UUID for one deploy/update/remove |
| `engine` | `docker`, `podman`, or `iofog` |
| `msUUID` | Microservice UUID |
| `containerId` | Runtime container ID |
| `sandboxId` | CRI pod sandbox ID (iofog) |
| `image` | Image reference used |
| `phase` | Sub-phase: pull, create, start, stop, remove |
| `result` | `ok`, `failed`, `skipped` |
| `durationMs` | Phase duration in milliseconds |
| `reasonCode` | Stable failure/drift code for alerting |
| `source` | `task`, `reconcile`, `shutdown`, `runtime_watch`, `api`, `watchdog` |
| `error` | Truncated error text (max 512 chars) |

**Never logged:** registry passwords, env values, JWT/bearer tokens.

## Event catalog

### ProcessManager / ContainerManager (typically Info)

| Event | When |
|-------|------|
| `pm.started` | Process Manager started |
| `pm.stopped` | Process Manager stopped |
| `task.enqueued` | Task added to queue |
| `task.started` | Task execution began |
| `task.completed` | Task succeeded |
| `task.failed` | Task failed after max retries |
| `task.retry` | Task re-queued after failure |
| `reconcile.cycle` | Monitor tick summary |
| `reconcile.decision` | Reconcile scheduled ADD/UPDATE/REMOVE |
| `container.pulling` | Image pull started |
| `container.pull.completed` | Image pull finished |
| `container.creating` | Container create started |
| `container.created` | Container created (pre-start) |
| `container.starting` | Container start invoked |
| `container.started` | Container running |
| `container.stopping` | Stop invoked |
| `container.stopped` | Container stopped |
| `container.removing` | Remove invoked |
| `container.removed` | Container removed |
| `container.update.phase` | Update flow phase (pull/remove/create) |
| `container.drift` | DB/runtime ID mismatch cleanup |
| `shutdown.drain` | Shutdown container drain |

### Engine (typically Debug)

| Event | When |
|-------|------|
| `engine.init` | Engine initialized |
| `engine.cri.sandbox.created` | CRI pod sandbox created (iofog) |
| `engine.cri.container.created` | CRI workload container created |
| `engine.container.start` | Engine start API |
| `engine.container.stop` | Engine stop API |
| `engine.container.remove` | Engine remove API |
| `engine.image.pulled` | Image pull via engine |
| `engine.prune` | Prune operation summary |

### Runtime watch (Info)

| Event | When |
|-------|------|
| `container.runtime.event` | External runtime signal for **iofog-labeled** workloads (`source=runtime_watch`) |

**Sources**

| Engine | Mechanism |
|--------|-----------|
| **docker** / **podman** | `pkg/docker/events.go` — Docker event stream (`die`, `start`, `destroy`, `oom`, …) |
| **iofog** | `pkg/engine/iofog/engine_runtime_events.go` — containerd subscribe (`/tasks/exit`, `/tasks/oom`) with CRI reason/`exitCode` enrichment |

**iofog fields on exit/OOM:** `runtimeStatus` (`exit` or `oom`), `exitCode`, `reason` (from CRI status when available), plus `containerId`, `msUUID`, `sandboxId`.

**Note:** ProcessManager may also emit `container.stopped` (or other CM lifecycle events) when it stops a workload via task/reconcile (`source=task` or `source=reconcile`). That is intentional — runtime watch reflects **external** or **async runtime** signals (e.g. `docker kill`, containerd task exit), while CM events reflect **agent-initiated** lifecycle. Both may appear for the same workload; use `source` to distinguish.

## Reason codes

| Code | Meaning |
|------|---------|
| `PULL_FAILED` | Registry pull failed |
| `PULL_CACHE_FALLBACK` | Pull failed; using local cache |
| `CREATE_FAILED` | Container create failed |
| `START_FAILED` | Container start failed |
| `NON_RESTARTABLE_EXIT` | Terminal CRI exit; recreate required |
| `STOP_FAILED` | Stop failed |
| `REMOVE_FAILED` | Remove failed |
| `RUNTIME_DRIFT` | Stale DB vs runtime |
| `STUCK_IN_RESTART` | Repeated start failures |
| `TASK_EXHAUSTED_RETRIES` | Task failed 5 times |
| `SHUTDOWN_DRAIN_TIMEOUT` | Shutdown drain deadline exceeded |

## Log levels

| Level | Use |
|-------|-----|
| **info** | Lifecycle transitions operators need at default `LOG_LEVEL=info` |
| **warn** | Degraded but continuing (cache fallback, stop warning before remove) |
| **error** | Operation failed |
| **debug** | Reconcile internals, CRI substeps, engine API detail |

## `LOG_LEVEL`: info vs debug

Set via environment variable or agent config (e.g. `LOG_LEVEL=info` in deployment). Values are case-insensitive (`info`, `INFO`, `debug`, …).

### Use `LOG_LEVEL=info` (production default)

Operators and SRE should run **info** in production. At this level you get:

- Full **deploy/remove story**: `task.*`, `container.pulling` → `container.started` / `container.stopped` → `container.removed`
- **Reconcile intent**: `reconcile.decision`, `reconcile.cycle` (one summary line per monitor tick)
- **External runtime**: `container.runtime.event` (`source=runtime_watch`) for docker/podman/iofog
- **Failures**: `task.failed`, `task.retry`, Warn/Error with `reasonCode`

You do **not** see: per-tick reconcile Debug spam, `engine.container.*` API timing, CRI substep Debug (`criStopContainer`, …), or legacy module `Debugf` strings in `pkg/docker` (pull progress, etc.) unless they use a non-`runtimeops` path at Info/Warn.

### Use `LOG_LEVEL=debug` (troubleshooting only)

Enable debug briefly when investigating:

- Engine API latency (`engine.container.start|stop|remove`, `durationMs`)
- iofog CRI teardown order and substep timing
- Reconcile monitor boundaries (`Start Monitoring containers` module Debug)
- Docker event stream Debug lines for **unlabeled** containers

**Guidance:** reproduce the issue at debug for a short window, capture logs, then return to **info**. Debug materially increases volume (see [Log volume](#log-volume)).

**Optional debug sampling:** set environment variable `LOG_DEBUG_SAMPLE_RATE` to a fraction in `0.0`–`1.0` (e.g. `0.1` keeps ~10% of `runtimeops` Debug lines in the log file). Unset or invalid → `1.0` (no sampling). Test sinks and metrics hooks are not sampled. Does not affect Info/Warn/Error events.

### Quick reference

| Question | Level |
|----------|-------|
| When did workload X start/stop? | **info** — `container.started` / `container.stopped` |
| Why did reconcile schedule an update? | **info** — `reconcile.decision` |
| Was the Docker API slow? | **debug** — `engine.container.*` + `durationMs` |
| Did containerd see OOM before PM? | **info** — `container.runtime.event` with `runtimeStatus=oom` |

## Layer ownership (docker / podman / iofog)

| Layer | Responsibility |
|-------|----------------|
| **ContainerManager** (`internal/processmanager`) | Info lifecycle intent: `container.started`, `container.removed`, etc. |
| **`pkg/engine` decorator** (`NewLoggingEngine`, docker/podman only) | Debug/Warn **engine API** calls: `engine.container.start`, `engine.container.stop`, `engine.container.remove`, `engine.image.pulled`. Includes `containerId`, `sandboxId` (when available), `durationMs`, and `engine` on **success (Debug) and failure (Warn)**. |
| **`pkg/docker`** | **Silent** on create/start/stop/remove/kill — no lifecycle Debug lines. The decorator wraps `pkg/engine/docker` after `Init()`; adding logs here would duplicate the decorator. |
| **`pkg/engine/iofog`** | Same event names as the decorator, emitted inside the iofog engine (CRI substeps, teardown order on remove). Not wrapped by the decorator. |

**Pull/network/exec in `pkg/docker`:** `image.go`, `network.go`, and `exec.go` may still use the module logger for non-lifecycle detail (e.g. pull progress). That is separate from container lifecycle; operators use `engine.image.pulled` (decorator or iofog) for engine-level pull completion.

**Correlation:** `operationId` is set by ProcessManager/ContainerManager context, not by `pkg/docker`.

## Example log line (Info)

```json
{
  "timestamp": "2026-05-19T12:00:01.234Z",
  "level": "info",
  "module": "Container Manager",
  "message": "container started",
  "event": "container.started",
  "operationId": "550e8400-e29b-41d4-a716-446655440000",
  "engine": "iofog",
  "msUUID": "abc-def-123",
  "containerId": "a1b2c3d4",
  "sandboxId": "sandbox-xyz",
  "image": "myapp:1.0",
  "result": "ok",
  "durationMs": 842,
  "source": "task"
}
```

## Query examples (Loki / Splunk)

Replace `<MS_UUID>`, `<OPERATION_ID>`, and label selectors (`job`, `index`, `host`) for your environment. JSON field names match logrus output (`event`, `msUUID`, `operationId`, …).

### 1. When did msUUID X start?

**Loki (LogQL)**

```logql
{job="iofog-agent"} | json | event="container.started" | msUUID="<MS_UUID>"
```

**Splunk (SPL)**

```spl
index=iofog host=* source="*iofog-agent*"
| spath
| search event=container.started msUUID="<MS_UUID>"
| table _time, msUUID, containerId, operationId, durationMs, engine, source
```

Also check agent-initiated vs external: `source=task` (normal deploy) vs `source=runtime_watch` with `container.runtime.event` and `runtimeStatus=start` (docker).

### 2. When did msUUID X stop?

**Loki**

```logql
{job="iofog-agent"} | json | event="container.stopped" | msUUID="<MS_UUID>"
```

**Splunk**

```spl
index=iofog host=* source="*iofog-agent*"
| spath
| search event=container.stopped msUUID="<MS_UUID>"
| table _time, msUUID, containerId, operationId, durationMs, reasonCode, source
```

For **external** stop/kill, also query runtime watch:

```logql
{job="iofog-agent"} | json | event="container.runtime.event" | msUUID="<MS_UUID>" | runtimeStatus=~"die|exit|oom|stop|destroy"
```

(`runtimeStatus` values depend on engine: docker uses `die`, iofog uses `exit` / `oom`.)

### 3. Failed tasks in the last hour

**Loki**

```logql
{job="iofog-agent"} | json | event="task.failed" | __error__="" 
```

Add a time window in Grafana Explore or:

```logql
sum(count_over_time({job="iofog-agent"} | json | event="task.failed" [1h]))
```

Include retries leading to failure:

```logql
{job="iofog-agent"} | json | event=~"task\\.(failed|retry)" | __error__=""
```

**Splunk**

```spl
index=iofog host=* source="*iofog-agent*"
| spath
| search event=task.failed _time>=relative_time(now(),"-1h@h")
| table _time, msUUID, operationId, reasonCode, error, engine
```

### 4. Slow container stops (`durationMs`)

Surfaces stops where the stop phase exceeded a threshold (adjust ms as needed).

**Loki**

```logql
{job="iofog-agent"} | json | event="container.stopped" | durationMs > 5000
```

Engine-layer stop API (debug only unless scraped at debug):

```logql
{job="iofog-agent"} | json | event="engine.container.stop" | durationMs > 3000
```

**Splunk**

```spl
index=iofog host=* source="*iofog-agent*"
| spath
| search event=container.stopped durationMs>5000
| sort -durationMs
| table _time, msUUID, containerId, durationMs, operationId, reasonCode
```

### 5. Correlate one deploy by `operationId`

Copy `operationId` from any line in the chain (`task.enqueued`, `task.started`, `container.started`, …).

**Loki**

```logql
{job="iofog-agent"} | json | operationId="<OPERATION_ID>"
```

**Splunk**

```spl
index=iofog host=* source="*iofog-agent*"
| spath
| search operationId="<OPERATION_ID>"
| sort _time
| table _time, event, level, msUUID, phase, result, durationMs, reasonCode, message
```

### Additional useful queries

**Non-restartable exits (recreate required)**

```logql
{job="iofog-agent"} | json | reasonCode="NON_RESTARTABLE_EXIT"
```

**Reconcile scheduled a change**

```logql
{job="iofog-agent"} | json | event="reconcile.decision" | msUUID="<MS_UUID>"
```

## Support bundle: `grep` / `jq` on agent logs

Default rotated files: `/var/log/iofog-agent/iofog-agent.0.log` (active), `iofog-agent.1.log`, … Adjust path if `logDirectory` is overridden in config.

```bash
LOG_DIR="${LOG_DIR:-/var/log/iofog-agent}"
ACTIVE_LOG="$LOG_DIR/iofog-agent.0.log"

# All runtimeops-style events for one microservice (compact)
grep '"msUUID":"<MS_UUID>"' "$ACTIVE_LOG" \
  | jq -c 'select(.event != null) | {ts:.timestamp, level, event, result, durationMs, source, operationId, reasonCode}'

# One correlated deploy / task chain
grep '"operationId":"<OPERATION_ID>"' "$ACTIVE_LOG" | jq -s 'sort_by(.timestamp)'

# Failed tasks (any rotated file in directory)
grep -h '"event":"task.failed"' "$LOG_DIR"/iofog-agent.*.log \
  | jq -c '{ts:.timestamp, msUUID, operationId, reasonCode, error}'

# Reconcile decisions in last N lines (quick tail)
tail -n 5000 "$ACTIVE_LOG" | grep '"event":"reconcile.decision"' | jq -c .

# External runtime events (docker kill, containerd OOM, …)
grep '"event":"container.runtime.event"' "$ACTIVE_LOG" \
  | jq -c '{ts:.timestamp, msUUID, containerId, runtimeStatus, exitCode, reason, source}'

# Slow stops (>5s) without Loki
grep '"event":"container.stopped"' "$ACTIVE_LOG" \
  | jq -c 'select(.durationMs != null and .durationMs > 5000)'

# Count reconcile cycles per hour (volume sanity check)
grep '"event":"reconcile.cycle"' "$ACTIVE_LOG" | jq -r '.timestamp' | cut -c1-13 | uniq -c
```

If `jq` is unavailable, `grep` alone still works:

```bash
grep '"event":"container.started"' "$ACTIVE_LOG" | grep '"msUUID":"<MS_UUID>"'
```

## Log volume

Structured logging adds predictable Info volume. Use this section for capacity planning and rotation sizing. Numbers are **order-of-magnitude** for a healthy agent; soak-test your fleet.

### `reconcile.cycle` (throttled)

`reconcile.cycle` is emitted at Info **only when**:

1. **Scheduling activity** in that tick: `scheduledAdd + scheduledUpdate + scheduledRemove > 0`, or
2. **Heartbeat:** every `logReconcileCycleEveryNTicks` monitor iterations when idle (default **60** in the default profile).

Configure via profile property `logReconcileCycleEveryNTicks` (minimum 1). Monitor tick interval is `monitorContainersStatusFreqSeconds` (automatic, often 5s).

**Idle steady-state (default):** with interval 5s and `logReconcileCycleEveryNTicks=60`, about **one** `reconcile.cycle` line per **5 minutes** (~12/hour), not every tick.

**Active reconcile:** any tick that schedules ADD/UPDATE/REMOVE still emits immediately (alongside `reconcile.decision` lines).

Fields: `desiredCount`, `scheduledAdd`, `scheduledUpdate`, `scheduledRemove`, `runningCount`, `durationMs`.

### Expected Info lines per workload operation

Approximate **runtimeops Info** lines per successful path (same `operationId` within one task):

| Operation | Typical Info events |
|-----------|---------------------|
| **Add (deploy)** | `task.enqueued`, `task.started`, `container.pulling`, `container.pull.completed`, `container.creating`, `container.created`, `container.starting`, `container.started`, `task.completed` → **~9 lines** |
| **Update** | Task chain + up to three `container.update.phase` (pull/remove/create) + create/start chain → **~12–15 lines** |
| **Remove** | `task.enqueued`, `task.started`, `container.stopping`, `container.stopped`, `container.removing`, `container.removed`, `task.completed` → **~7 lines** |
| **Reconcile-driven schedule** | +1 `reconcile.decision` per scheduled ADD/UPDATE/REMOVE (no CM lifecycle until task runs) |

**Not counted above (by design):**

- `reconcile.cycle` (background tick)
- `container.runtime.event` (only on external/async runtime signals)
- Debug engine/CRI lines (only if `LOG_LEVEL=debug`)

### Rotation guidance

- Size `logFileCount` and max file size so **at least 24–72 hours** of **info** retention fit your worst-case microservice count and reconcile interval.
- Temporarily raising Debug on a single node can multiply volume 5–10×; avoid fleet-wide debug.

## Metrics (deferred follow-up milestone)

<a id="metrics-deferred-follow-up-milestone"></a>

**PR-5a (Prometheus metrics) is deferred** to a separate follow-up milestone. This logging initiative ships **logs only**; do not block releases on metric dashboards.

**Planned follow-up (PR-5a scope):**

- Counters/histograms from `runtimeops.Emit` via `SetMetricsHook` (stub exists in `internal/runtimeops/emit.go`)
- Example series: `iofog_runtime_operations_total{event,result,engine}`, `iofog_runtime_operation_duration_seconds{event,engine}`
- Implementation options: `prometheus/client_golang` or extend `internal/localapi/handlers/metrics.go`

**Follow-up issue template (copy into tracker):**

```markdown
## Runtime logging metrics (PR-5a follow-up)

**Depends on:** Enterprise runtime logging (PR-0 … PR-5b) merged.

**Goal:** Export Prometheus metrics from `runtimeops.Emit` without duplicating log volume.

**Acceptance criteria:**
- [ ] `SetMetricsHook` wired to Prometheus registry
- [ ] Counter `iofog_runtime_operations_total` labels: event, result, engine
- [ ] Histogram `iofog_runtime_operation_duration_seconds` labels: event, engine
- [ ] Documented scrape path or exposed on existing local API metrics handler
- [ ] Dashboard panels linked from docs/logging.md

**Out of scope:** Changing log event schema or LOG_LEVEL defaults.
```

Until PR-5a lands, use Loki/Splunk queries in this doc and support-bundle `jq`/`grep` for operations.

## Example engine Warn on API failure (decorator)

When `StartContainer` fails, the decorator emits one Warn line with `containerId` (no duplicate log from `pkg/docker`):

```json
{
  "level": "warn",
  "module": "ContainerEngine",
  "message": "container start failed",
  "event": "engine.container.start",
  "engine": "docker",
  "containerId": "cid-err",
  "reasonCode": "START_FAILED",
  "result": "failed",
  "durationMs": 12,
  "error": "start failed"
}
```

## Implementation status

| PR | Status |
|----|--------|
| PR-0 | `internal/runtimeops` foundation |
| PR-1a–PR-2b | ProcessManager task/reconcile/shutdown + CM lifecycle |
| PR-3a | `pkg/engine` logging decorator (docker/podman) |
| PR-3b | `pkg/engine/iofog` CRI/runtimeops logging |
| PR-3c | Documented: decorator owns lifecycle Debug; `pkg/docker` lifecycle APIs stay silent |
| PR-4a | `pkg/docker/events.go` → Info `container.runtime.event` for labeled containers |
| PR-4b | `pkg/engine/iofog` containerd `/tasks/exit` and `/tasks/oom` → Info `container.runtime.event` |
| PR-5a | **Deferred** — Prometheus metrics ([follow-up](#metrics-deferred-follow-up-milestone)) |
| PR-5b | Operations runbook (this doc): queries, LOG_LEVEL, support bundle, log volume |
| PR-6a | `reconcile.cycle` throttling + `LOG_DEBUG_SAMPLE_RATE` for Debug |
| PR-6+ | OTel stretch (see enterprise logging plan) |
