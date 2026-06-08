# Network Manager

The network interface manager detects the **host IP and primary interface** used for agent identity and networking features. It runs an initial detection at start and refreshes periodically.

**Code:** `internal/network/`

## Purpose

- Select IOFog/Edgelet network interface per config rules
- Track current IP address and hostname
- Provide gateway IP helpers for DNS resolver scopes
- Periodic refresh (30 minutes) of interface state

## Dependencies

| Depends on | Reason |
|------------|--------|
| `config` | Interface selection preferences |

| Used by | Reason |
|---------|--------|
| `supervisor` | Started before Field Agent |
| `dnsresolver` | `GatewayIPForScope()` for listener bind addresses |
| `processmanager` | Network setup for workloads |
| `runtimeapi` | Status/info enrichment |

## Lifecycle

### Start

1. `UpdateNetworkInterface()` — immediate detection
2. On failure, `Start()` retries recursively (logs error)
3. `periodicUpdate()` goroutine every **30 minutes**

### Stop

Cancel context.

## Key APIs

| Function | Role |
|----------|------|
| `GetCurrentIPAddress()` | Agent IP for status |
| `GetNetworkInterfaceInfo()` | Interface + address struct |
| `GatewayIPForScope` | Used by embedded DNS |

## Configuration

Interface selection driven by config fields (controller-facing IP discovery). See `internal/config` for `NetworkInterface` related settings in default profiles.

## External APIs

No dedicated EdgeletAPI routes; IP/interface appear in system status/info payloads.

## Observability

- Log module: `"Network Interface Manager"`
- Errors on failed interface update logged at start

## Failure modes

| Symptom | Typical cause |
|---------|----------------|
| Start loop / stack risk | Repeated `UpdateNetworkInterface` failure (recursive retry) |
| Wrong agent IP on controller | Interface selection mismatch on multi-homed host |
| DNS bind failures | Gateway IP not available for scope |

## Code map

| File | Role |
|------|------|
| `manager.go` | Detection, periodic update, gateway helpers |

Related: [dnsresolver.md](dnsresolver.md), [statusreporter.md](statusreporter.md).
