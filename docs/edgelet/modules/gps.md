# GPS Manager

The GPS manager resolves **agent geographic coordinates** for Controller reporting and optional NMEA/device integration. It supports AUTO, DYNAMIC, MANUAL, and OFF modes with async initialization.

**Code:** `internal/gps/`

## Purpose

- Determine coordinates from GPS device, web geolocation, or manual config
- Expose GPS state for status and Controller POST
- Serve optional web-based geolocation handler (dynamic mode)
- Schedule periodic coordinate updates

## Dependencies

| Depends on | Reason |
|------------|--------|
| `config` | GPS mode, manual coordinates, device paths |
| `fieldagent` | GPS config sync callback on reload |

| Used by | Reason |
|---------|--------|
| `supervisor` | Started before EdgeletAPI |
| `edgeletapi` | `GET/POST /v1/system/gps` |
| `fieldagent` | `InstanceGPSConfigUpdated` on config reload |

## Modes

| Mode | Behavior |
|------|----------|
| `OFF` | No coordinate collection |
| `MANUAL` | Static coordinates from config |
| `DYNAMIC` | Web/geolocation handler |
| `AUTO` | Device NMEA with fallback chain |

Implementation split: `device.go`, `nmea/`, `web.go`.

## Lifecycle

### Start

1. Reset context (supervisor restart safe)
2. `startCoordinateUpdateScheduler()`
3. Async `initializeGps()` in goroutine — failures fall back to OFF mode

### Stop

Cancel scheduler and main context.

## Configuration

GPS settings in config profiles and Controller-pushed fog config. Field Agent registers `SetGPSConfigCallback` on Supervisor for dedicated sync.

EdgeletAPI allows operator read/update via `/v1/system/gps`.

## Module status

| Property | Value |
|----------|-------|
| StatusReporter index | `6` (`utils.GPSManager`) |

Internal `gps.Status` tracks health, coordinates, mode, last update.

## External APIs

| Route | Role |
|-------|------|
| `GET /v1/system/gps` | Read GPS state |
| `POST /v1/system/gps` | Update manual/dynamic settings |

Controller sync via Field Agent fog config POST.

## Observability

- Log module: `"GPS Manager"`
- Health status enum on internal `Status` object

## Failure modes

| Symptom | Typical cause |
|---------|----------------|
| OFF mode after start | Init error; device missing |
| Stale coordinates | Scheduler stopped; mode OFF |
| IP geolocation errors | Network blocked; sets health to IP error |

## Code map

| File | Role |
|------|------|
| `manager.go` | Mode scheduling, init, config reactions |
| `device.go` | Hardware GPS |
| `web.go` | Web geolocation |
| `nmea/` | Sentence parsing |

Related: [fieldagent.md](fieldagent.md), [edgeletapi.md](edgeletapi.md), [statusreporter.md](statusreporter.md).
