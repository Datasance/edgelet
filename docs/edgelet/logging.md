# Edgelet logging

Structured lifecycle logging for ProcessManager, container engines, and EdgeletAPI. Events are emitted via `internal/runtimeops` and written as **top-level JSON fields** in Edgelet daemon logs (logrus JSON formatter).

## Field schema

| Field | Description |
|-------|-------------|
| `timestamp` | Log time (logrus) |
| `level` | debug / info / warn / error |
| `module` | Logger module name (e.g. `Container Manager`, `Edgelet API Handler`) |
| `message` | Short human-readable summary |
| `event` | Stable event name (see catalog below) |
| `operationId` | Correlation UUID for one deploy/update/remove |
| `engine` | `docker`, `podman`, or `edgelet` |
| `msUUID` | Microservice UUID |
| `containerId` | Runtime container ID |
| `sandboxId` | CRI pod sandbox ID (edgelet engine) |
| `image` | Image reference used |
| `phase` | Sub-phase: pull, create, start, stop, remove |
| `result` | `ok`, `failed`, `skipped` |
| `durationMs` | Phase duration in milliseconds |
| `reasonCode` | Stable failure/drift code for alerting |
| `source` | `task`, `reconcile`, `shutdown`, `runtime_watch`, `api`, `watchdog` |
| `error` | Truncated error text (max 512 chars) |

**Never logged:** registry passwords, env values, JWT/bearer tokens.

## EdgeletAPI access logs

EdgeletAPI emits dedicated access/reject events:

| Event | When |
|-------|------|
| `edgeletapi.access` | Authorized request (method, path, status, duration) |
| `edgeletapi.reject` | Auth or RBAC rejection |
| `edgeletapi.debug` | Verbose handler diagnostics (`LOG_LEVEL=debug`) |

Log lines use module `"Edgelet API Router"` or `"Edgelet API Server"`.

## ProcessManager event catalog

| Event | When |
|-------|------|
| `pm.started` / `pm.stopped` | Process Manager lifecycle |
| `task.enqueued` / `task.started` / `task.completed` / `task.failed` / `task.retry` | Task queue |
| `reconcile.cycle` / `reconcile.decision` | Monitor tick |
| `container.pulling` → `container.removed` | Container lifecycle phases |
| `container.runtime.event` | External runtime signal (`source=runtime_watch`) |
| `shutdown.drain` | Graceful shutdown |

Engine-level debug events: `engine.init`, `engine.cri.*`, `engine.container.*`, `engine.image.pulled`.

## Log levels

| Level | Use |
|-------|-----|
| **info** | Production default — lifecycle transitions, reconcile decisions, access logs |
| **warn** | Degraded but continuing |
| **error** | Operation failed |
| **debug** | Reconcile internals, CRI substeps, EdgeletAPI handler detail |

Set via config or environment (`LOG_LEVEL=info`).

## Log files (rotated)

When `logDirectory` is configured (default `/var/log/edgelet/`), control plane and data plane write **separate file series** in the same directory:

| systemd unit | Basename | Active file |
|--------------|----------|-------------|
| `edgelet.service` | `edgelet` | `edgelet.0.log` |
| `edgelet-containerd.service` | `edgelet-containerd` | `edgelet-containerd.0.log` |

Each series rotates independently using `logFileCount` and a share of **`logLimit`** (combined daemon budget):

| Mode | `edgelet` share | `edgelet-containerd` share |
|------|-----------------|----------------------------|
| Runtime split (`EDGELET_RUNTIME_SPLIT=1`, embedded engine) | 60% | 40% |
| Monolithic embedded, docker, podman, desktop | 100% | (unit not used) |

**Hot reload:** `logLevel`, `logLimit`, and `logFileCount` apply on config reload without rotating the active log file. Control plane reloads via SIGHUP / `edgelet system reload`; data plane reloads via `systemctl kill -s HUP edgelet-containerd`. Changing `logDirectory` still requires a process restart.

Microservice logs under `logDirectory/containers/` use separate per-UUID series (not included in the daemon 60/40 split).

## journald

On systemd hosts:

```bash
sudo journalctl -u edgelet -f
sudo journalctl -u edgelet-containerd -f
sudo journalctl -u edgelet --since "30 min ago" | jq -r 'select(.event!=null) | .event'
```

## Example queries

Failed tasks (Loki-style):

```
{job="edgelet"} | json | event="task.failed"
```

Correlate one deploy operation:

```
{job="edgelet"} | json | operationId="<OPERATION_ID>"
```

EdgeletAPI rejections:

```
{job="edgelet"} | json | event="edgeletapi.reject"
```

## Reason codes

| Code | Meaning |
|------|---------|
| `PULL_FAILED` | Registry pull failed |
| `CREATE_FAILED` | Container create failed |
| `START_FAILED` | Container start failed |
| `NON_RESTARTABLE_EXIT` | Terminal CRI exit; recreate required |
| `RUNTIME_DRIFT` | Stale DB vs runtime |
| `TASK_EXHAUSTED_RETRIES` | Task failed 5 times |

Full list in source: `internal/runtimeops`.
