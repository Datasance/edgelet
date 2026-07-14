# Microservice exec sessions (operator guide)

Interactive exec into microservice containers uses a **multi-session model** aligned with Controller **v3.8.x** multi-exec sessions. Each session has its own id, WebSocket attach path, and runtime exec process.

## Overview

| Concern | Controller-initiated exec | Local `edgelet ms exec` |
|---------|---------------------------|-------------------------|
| Who opens the session | Pot user / EdgeOps via Controller | Operator CLI on the node |
| Session discovery | Field agent polls `GET /api/v3/agent/exec/sessions` | `POST /v1/ms/{id}/exec/sessions` |
| Attach transport | Agent WebSocket `…/exec/microservice/{uuid}/{sessionId}` | `GET /v1/ms/{id}/exec/sessions/{sessionId}:attach` |
| Concurrent limit | **3 per microservice** (Controller quota) | **Unlimited** (node resources only) |
| Reported in status | Yes — `execSessionIds[]` on agent status | No — local sessions are excluded from Pot status |

Exec sessions are modeled like **log sessions**: poll a session list, attach per `sessionId`, reconcile when rows disappear.

See also: [edgelet-api-v1.md](edgelet-api-v1.md) (local API routes), [modules/fieldagent.md](modules/fieldagent.md) (controller bridge), [modules/processmanager.md](modules/processmanager.md) (runtime registry).

## Local exec (`edgelet ms exec`)

```bash
edgelet ms exec <uuid|namespace.name>
edgelet ms exec <uuid> -- /bin/sh -c 'ps aux'
```

Flow:

1. CLI calls `POST /v1/ms/{id}/exec/sessions` with the remote command (default interactive shell).
2. EdgeletAPI waits up to **15 seconds** for the shell to start before returning.
3. CLI attaches over WebSocket and streams stdin/stdout/stderr until the remote command exits.

Multiple local sessions on the same microservice can run concurrently. Each session gets a unique runtime exec id (`{containerID[:12]}-local-{uuid[:8]}`) so local CLI exec does not collide with controller sessions.

### Start timeout

If the shell does not become ready within 15 seconds, the API returns HTTP **504** with code **`EXEC_START_TIMEOUT`**. The CLI surfaces:

```text
Error[EXEC_START_TIMEOUT]: Interactive shell did not start within 15 seconds. Retry `edgelet ms exec`; if the problem persists, check microservice and engine logs.
```

Common causes: container not running, image missing `/bin/sh`, engine overload, or stale containerd exec state after a prior crash.

## Controller-initiated exec

When a user opens exec from Pot / EdgeOps:

1. Controller creates a session row and sets `execSessions: true` on the next `config/changes`.
2. The field agent polls the session list and opens one agent WebSocket per pending row.
3. MessagePack relay frames use **`execId` = `sessionId`** (not containerd runtime ids).
4. When the session row disappears from the poll, the agent tears down that attachment only.

The legacy single-session path (`WS /agent/exec/{microserviceUuid}` with init MessagePack pairing) is removed in **v1.0.0-rc.6**.

### Status reporting

Agent status includes controller attachment session ids per microservice:

```json
"execSessionIds": ["session-uuid-1", "session-uuid-2"]
```

Values are **controller session ids** for attachments this agent holds — not containerd exec process names and not local CLI sessions.

## `execEnabled` (deprecated)

Edgelet **no longer** uses the microservice `execEnabled` flag to open or keep exec sessions. Controller multi-exec session poll (`execSessions: true`) drives attach and teardown.

The SQLite column may still exist on upgraded databases; removal from the microservice model is an application-level cleanup with **no schema migration** in this release. Operators do not need to toggle `execEnabled` for exec to work.

## Related operations

- **Control plane restart** stops interactive exec on the controller microservice before bounce — see [control-plane.md](control-plane.md#restart).
- **Health checks** use a separate short-lived exec path (`ExecWithExitCode`) and are not part of the interactive session registry.

## Troubleshooting

See [troubleshooting.md](troubleshooting.md#microservice-exec) for timeout, orphan session, and concurrent-session scenarios.
