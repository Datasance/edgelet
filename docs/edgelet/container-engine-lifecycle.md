# Container engine lifecycle (Plan 9A)

Operator guide for **`containerEngine`** and **`dockerUrl`** changes. Contract: [ENGINE-LIFECYCLE.md](../../.cursor/edgelet/ENGINE-LIFECYCLE.md).

## Change classes

| Class | Keys | Action |
|-------|------|--------|
| Hot | frequencies, log level, controller URL, GPS, etc. | SIGHUP reload in-process |
| Warm | `dockerUrl` with same `docker`/`podman` engine | Reconnect socket — **no** restart |
| Cold | `containerEngine` | Quiesce + MS cleanup + **`pendingRestart`** + **restart required** |

## Cold engine change

```bash
edgelet config --ce docker
# or PATCH /v1/system/config {"set":{"containerEngine":"docker"}}
edgelet system status | grep runtime.pendingRestart   # true
sudo systemctl restart edgelet
```

After restart:

- New engine is active (`runtime.engine`, `runtime.engineReady`)
- Microservice **spec rows** in SQLite are kept; containers are **recreated** on the new engine
- Images/volumes on the **old** engine are **lost** (not migrated)

## Warm dockerUrl reload (docker/podman)

```bash
edgelet config -c unix:///var/run/docker.sock
```

On failure the daemon **reverts YAML** to the last-known-good URL and keeps the existing client.

## Linux startup

Thin **`edgelet daemon`** always **execs the fat runtime** at `/var/lib/edgelet/data/current/bin/edgelet` (Plan 6/7 two-layer layout). The supervisor and engines run in fat only — never in the thin wrapper.

| Engine | Thin `edgelet daemon` | Fat runtime after exec |
|--------|------------------------|-------------------------|
| `edgelet` | `EnsureExtracted` (if needed) → exec fat | Bootstrap embedded containerd |
| `docker` / `podman` | Exec fat when already on disk; **skip full extract** if bundle present | Connect external socket (retry at boot) |

First start on a node with **no** extracted bundle (any engine) runs `EnsureExtracted` once so fat exists on disk.

Switching **away** from `edgelet` stops orphaned embedded containerd before exec; the extract tree remains on disk.

## Status fields

`GET /v1/system/status` includes:

- `runtime.engine`
- `runtime.dockerUrl`
- `runtime.pendingRestart`
- `runtime.engineReady`

## Integration tests

```bash
./test/engine-lifecycle/run-all.sh
```

See [test/engine-lifecycle/README.md](../../test/engine-lifecycle/README.md).

Related: [container-engine.md](container-engine.md)
