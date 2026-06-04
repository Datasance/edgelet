# Container engine lifecycle

Operator guide for **`containerEngine`** and **`containerEngineUrl`** changes.

## Engine values

| `containerEngine` | Platform | `containerEngineUrl` |
|-------------------|----------|----------------------|
| `edgelet` | linux only | **Fixed** `unix:///run/edgelet/containerd.sock` — not user-editable |
| `docker` | linux, darwin, windows | Default `unix:///var/run/docker.sock` (auto-set on engine change) |
| `podman` | linux, darwin, windows | Default `unix:///run/podman/podman.sock` (auto-set on engine change) |

## Change classes

| Class | Keys | Action |
|-------|------|--------|
| Hot | frequencies, log level, controller URL, GPS, etc. | SIGHUP reload in-process |
| Warm | `containerEngineUrl` with same `docker`/`podman` engine | Reconnect socket — **no** restart |
| Cold | `containerEngine` | Quiesce + MS cleanup + **`pendingRestart`** + **restart required** |

**Never** hot-switch `containerEngine` at runtime.

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

## Warm containerEngineUrl reload (docker/podman)

```bash
edgelet config --cu unix:///var/run/docker.sock
```

On failure the daemon **reverts YAML** to the last-known-good URL and keeps the existing client.

## Linux startup

Thin **`edgelet daemon`** always **execs the fat runtime** at `/var/lib/edgelet/data/current/bin/edgelet`. The supervisor and engines run in fat only — never in the thin wrapper.

| Engine | Thin `edgelet daemon` | Fat runtime after exec |
|--------|------------------------|-------------------------|
| `edgelet` | `EnsureExtracted` (if needed) → exec fat | Bootstrap embedded containerd |
| `docker` / `podman` | Exec fat when already on disk; **skip full extract** if bundle present | Connect external socket (retry at boot) |

First start on a node with **no** extracted bundle (any engine) runs `EnsureExtracted` once so fat exists on disk.

Switching **away** from `edgelet` stops orphaned embedded containerd before exec; the extract tree remains on disk.

## External engine degraded mode

- Boot without socket: degraded + background recovery.
- Recovery reads **live config** each attempt.
- **Never** auto-fallback to embedded `edgelet`.

## Status fields

`GET /v1/system/status` includes:

- `runtime.engine`
- `runtime.containerEngineUrl`
- `runtime.pendingRestart`
- `runtime.engineReady`

## Out of scope

- QoS classes (Guaranteed/Burstable/BestEffort)
- Rootless edgelet
- Engine migration of images/volumes between engines

## Integration tests

```bash
./test/engine-lifecycle/run-all.sh
```

See [test/engine-lifecycle/README.md](../../test/engine-lifecycle/README.md).

Related: [container-engine.md](container-engine.md)
