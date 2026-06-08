# Resource Consumption Manager

The resource consumption manager samples **host CPU, memory, and disk** usage against configured limits and publishes results to StatusReporter. It also participates in limit enforcement signaling for the agent.

**Code:** `internal/resourceconsumption/`

## Purpose

- Poll system metrics via gopsutil (CPU, mem, disk)
- Compare against configured limits (bytes / CPU percentage)
- Update `ResourceConsumptionManagerStatus` on StatusReporter
- Collect initial sample immediately on start (no wait for first tick)

## Dependencies

| Depends on | Reason |
|------------|--------|
| `statusreporter` | Publish usage metrics |
| `config` | Limits and poll frequency |
| `gopsutil` | Host metrics |

| Used by | Reason |
|---------|--------|
| `supervisor` | Started early (before Field Agent) |

## Lifecycle

### Start

1. `InstanceConfigUpdated()` — load limits from config
2. `collectUsageData()` immediately
3. Start periodic worker at configured frequency

### Config update

`InstanceConfigUpdated()` refreshes disk/CPU/memory limits and may restart sampling logic.

## Configuration

Limits loaded from config profile (typical keys):

| Key | Unit |
|-----|------|
| Disk limit | bytes |
| Memory limit | bytes |
| CPU limit | percentage |

Exact YAML names match `config.yaml` profiles — see default config in `internal/config/default_config.yaml`.

## Module status

| Property | Value |
|----------|-------|
| StatusReporter index | `0` (`utils.ResourceConsumptionManager`) |
| First slot in `modulesStatus[]` | Resource Consumption |

Status fields include `memoryUsage`, `cpuUsage`, `diskUsage` (human-readable in logs on start).

## External APIs

Exposed indirectly via `GET /v1/system/status` resource section and Controller status POST.

## Observability

- Log module: `"Resource Consumption Manager"`
- Initial and periodic debug logs with MiB/GiB formatting

## Failure modes

| Symptom | Typical cause |
|---------|----------------|
| Zero usage reported | gopsutil error on platform |
| Limit warnings | Usage exceeded configured thresholds |

## Code map

| File | Role |
|------|------|
| `manager.go` | Sampling loop, limit comparison, status updates |

Related: [statusreporter.md](statusreporter.md), [supervisor.md](supervisor.md).
