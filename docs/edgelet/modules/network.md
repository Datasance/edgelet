# Network Manager

The network interface manager detects the **host IP and primary interface** used for agent identity and networking features. It runs an initial detection at start and refreshes periodically.

**Code:** `internal/network/`

## Purpose

- Select IOFog/Edgelet network interface per config rules
- Track current IP address and hostname
- Provide gateway IP helpers for DNS resolver scopes
- Periodic refresh (60s while IP unset, 30 minutes when IP is known)

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

1. **Sync tier** — up to **5** `UpdateNetworkInterface()` attempts with **200ms** spacing (≤~3s blocking)
2. On IP found: start periodic refresh goroutine; return nil
3. On sync failure: log WARN, leave empty IP, **return nil** (degraded continue — supervisor and Field Agent proceed)
4. **Async recovery** — when sync tier leaves IP empty, a background goroutine runs up to **10** more attempts (15 total) with exponential backoff (30s base, double each loop, 8 min cap)
5. **Periodic refresh** — **60s** while IP empty; **30 min** when IP is set; errors are logged and the same goroutine continues (no respawn)

### Stop

Cancel context (stops periodic update and async recovery).

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
| Empty IP at boot (`unable to retrieve ip address`) | No usable IPv4 during sync tier; agent continues in degraded mode and retries async + periodic |
| IP appears after boot delay | Far-edge link or DHCP late; async recovery or 60s periodic picks up address |
| Wrong agent IP on controller | Interface selection mismatch on multi-homed host |
| DNS bind failures | Gateway IP not available for scope |

## Code map

| File | Role |
|------|------|
| `manager.go` | Detection, periodic update, gateway helpers |

Related: [dnsresolver.md](dnsresolver.md), [statusreporter.md](statusreporter.md).
