# Resource Manager

The resource manager periodically sends **hardware and USB inventory** from the host HAL to the Controller via Field Agent. It is a lightweight polling module with no local persistence.

**Code:** `internal/resourcemanager/`

## Purpose

- On `deviceScanFrequency` interval, collect HW/USB info
- Delegate POST paths to Field Agent (`SendHWInfoFromHalToController`, `SendUSBInfoFromHalToController`)

## Dependencies

| Depends on | Reason |
|------------|--------|
| `fieldagent` | Controller REST transport |
| `config` | `deviceScanFrequency` |

| Used by | Reason |
|---------|--------|
| `supervisor` | Started after Process Manager |

## Lifecycle

### Start

`GetInstance().Start()`:

1. Capture Field Agent singleton
2. `startWorker()` — goroutine with ticker

### Config update

`InstanceConfigUpdated()` restarts worker with new frequency (cancels prior worker context).

### Stop

Cancel main context; `wg.Wait()`.

## Configuration

| Key | Effect |
|-----|--------|
| `deviceScanFrequency` | Seconds between HAL scan POSTs |

## Module status

| Property | Value |
|----------|-------|
| StatusReporter index | `5` (`utils.ResourceManager`) |
| `GetModuleIndex()` | Used by Supervisor startModule |

## External APIs

Outbound only — Controller REST via Field Agent. No EdgeletAPI routes.

## Observability

- Log module: `"Resource Manager"`
- Debug logs around each scan cycle

## Failure modes

| Symptom | Typical cause |
|---------|----------------|
| No HW info on controller | Not provisioned; Controller disconnected |
| Scan stops after reload | Worker restart failed — check logs |

## Code map

| File | Role |
|------|------|
| `manager.go` | Worker loop, HAL send delegation |

Related: [fieldagent.md](fieldagent.md), [statusreporter.md](statusreporter.md).
