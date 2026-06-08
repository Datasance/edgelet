# SSH Proxy Manager

The SSH proxy manager maintains an optional **reverse SSH tunnel** configured by the Controller. It exposes remote access to the agent host through Controller-directed proxy settings pushed via the change feed.

**Code:** `internal/proxy/`

## Purpose

- Open/close SSH tunnel based on Controller proxy config map
- Monitor connection health
- Update StatusReporter SSH proxy status (`OPEN`, `CLOSED`, `FAILED`)

## Dependencies

| Depends on | Reason |
|------------|--------|
| `statusreporter` | Tunnel status for system status |
| `config` | Implicit via proxy config payload |

| Used by | Reason |
|---------|--------|
| `fieldagent` | `proxy.GetInstance().Update(config)` from changes processing |

## Lifecycle

Not a Supervisor `Module` with `Start()` in the main sequence — **lazy singleton** updated when Controller sends proxy configuration.

### Update(config map)

1. Validate non-empty config
2. If connected and close flag set → close tunnel
3. If not connected → establish tunnel from config
4. Unexpected states logged with status `OPEN`/`FAILED`

Default ports in constants: local **22**, remote **9999** (overridden by config).

## Configuration

Proxy settings arrive dynamically from Controller (not static `config.yaml` keys). Processed in `fieldagent/changes.go` when proxy-related changes appear.

## External APIs

No EdgeletAPI routes. Status visible on:

- `GET /v1/system/status` → SSH proxy section
- Controller diagnostics/status POST

## Observability

- Log module: `"SSH Proxy Manager"`
- `SSHProxyManagerStatus` on StatusReporter

## Failure modes

| Symptom | Status | Cause |
|---------|--------|-------|
| Tunnel already open | `OPEN` | Duplicate open request |
| Invalid config | `FAILED` | Empty or malformed map |
| Connection drop | Monitoring loop | Network/firewall |

## Code map

| File | Role |
|------|------|
| `manager.go` | Update orchestration, monitoring |
| `connection.go` | SSH connection implementation |

Related: [fieldagent.md](fieldagent.md), [statusreporter.md](statusreporter.md).
