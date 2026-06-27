# Deadlock audit

Audit scope: `internal/volumemount/`, `internal/fieldagent/`, `internal/processmanager/` (DL-1).

## Summary

Plan 19 fixed two production deadlock/contention paths:

1. **LC-1 (19-A):** Re-entrant `VolumeMountManager.indexLock` when `ProcessVolumeMountChanges` → `updateVolumeMount` → `syncMicroserviceSymlinks` re-acquired the same lock (CONFIGMAP/SECRET version bump froze the changes worker).
2. **LC-3 (19-C):** `Clear()` held `indexLock` across slow filesystem walks, blocking PM `CleanupMicroserviceVolumes()` during background deprovision.

No additional re-entrant mutex deadlocks were found in the audit packages. Lock ordering for `indexLock` → `typeCacheLock` is consistent (never reversed). `provisioningMu` is released before background deprovision cleanup runs; PM monitor and volumemount cleanup operate on separate locks.

**19-D (LC-4):** `runChangesWorker` has top-level and per-tick `recover()` plus a 5-minute `processChanges` timeout so the worker resumes polling after panic or slow work.

**22-C (BR-C7):** `controllerReconcile` holds `reconcileMu` for the full Pot reload chain (registries → volume mounts → microservices). The ping worker starts reconcile in a background goroutine with `recover()` so the ping timer is not blocked. `runChangesWorker` / `processChanges` do not acquire `reconcileMu`; init-path dedup uses `shouldSkipInitReload()` (reads `reconnect.mu` + `state.IsInitialization()` without holding `reconcileMu`). Volume mount LC-1: reconcile calls the same `loadVolumeMounts` → `ProcessVolumeMountChanges` path as getChanges; no new nested `indexLock` pattern. Lock order when both are touched: `reconcileMu` → short `reconnect.mu` write at reconcile completion; never `reconnect.mu` → `reconcileMu`.

## Mutex inventory

### `internal/volumemount/`

| Mutex | File | Acquired by | Calls children that re-acquire | Notes |
|-------|------|-------------|--------------------------------|-------|
| `indexLock` (`sync.RWMutex`) | `manager.go` | `loadFromStore`, `ProcessVolumeMountChanges`, `trackMicroserviceUsage`, `syncMicroserviceSymlinks`, `GetVolumeMountByName`, `CleanupMicroserviceVolumes` (brief), `Clear`, `ClearControllerArtifacts`, `rebuildTypeCache` | **Fixed (19-A):** `updateVolumeMount` now calls `syncMicroserviceSymlinksUnsafe` instead of public `syncMicroserviceSymlinks` | `deleteVolumeMount` / `createVolumeMount` run under `ProcessVolumeMountChanges` lock; use `typeCacheLock` only, not nested `indexLock` |
| `typeCacheLock` (`sync.RWMutex`) | `manager.go` | `rebuildTypeCacheUnsafe`, `clearVolumeMountStateLocked`, `deleteVolumeMount`, `GetVolumeMountType`, `rebuildTypeCache` | None on `indexLock` | Always acquired **after** `indexLock` when both are held |

| `*Unsafe` / locked helper | Lock precondition |
|---------------------------|-------------------|
| `rebuildTypeCacheUnsafe` | Caller holds `indexLock` (read or write) |
| `trackMicroserviceUsageUnsafe` | Caller holds `indexLock` (read or write) |
| `syncMicroserviceSymlinksUnsafe` | Caller holds `indexLock` (read or write) |
| `cleanupMicroserviceVolumesIndexUnsafe` | Caller holds `indexLock` (write) |
| `clearVolumeMountStateLocked` | Caller holds `indexLock` (write) |

**Lock order:** `indexLock` → `typeCacheLock` only. No path acquires `typeCacheLock` then `indexLock`.

**Concurrent paths (safe):** `PrepareMicroserviceVolumeMount` / `ResolveHostPath` call `trackMicroserviceUsage` (acquires `indexLock` independently) from PM container create — orthogonal to `ProcessVolumeMountChanges` on different goroutines.

### `internal/fieldagent/`

| Mutex | File | Acquired by | Re-entrancy risk |
|-------|------|-------------|------------------|
| `fa.mu` | `agent.go` | `Update`, API client lifecycle, state helpers | No nested self-lock |
| `provisioningMu` | `agent.go` | `Provision`, `DeprovisionWithScope` (`TryLock` on deprovision) | Released when `Deprovision` returns; background cleanup does **not** hold it |
| `microservicesMu` | `agent.go`, `microservice_manager.go` | MS list CRUD / reads | PM reconcile reads via `GetLatestMicroservices`; no FA lock during PM `microserviceManager` internal locks |
| `containerConfigMu` | `agent.go`, `sync.go` | Config push paths | Independent of volumemount locks |
| `execSessionsMu` | `agent.go`, exec paths | Exec session map | Independent |
| `bootstrapMu` | `bootstrap_sync.go` | Cache-loaded flag | Independent |
| `reconcileMu` | `reconnect_reconcile.go` | `controllerReconcile` single-flight | Does not nest `state.mu`; see Plan 22-C section |
| `reconnect.mu` | `reconnect_reconcile.go` | Connect generation + last reconcile metadata | Independent of `reconcileMu`; lock order: `reconcileMu` then brief `reconnect.mu` at end of reconcile |
| `state.mu` | `state.go` | Controller status / init flags | `shouldSkipInitReload` reads init under `reconnect.mu` only; reconcile reads init without holding `reconcileMu` across Pot HTTP |
| `controllerRegister.mu` | `controller_register.go` | Register retry state | Independent |

WebSocket / log-session mutexes (`connMu`, `writeMu`, `lsm.mu`, `exec_callback.mu`) are per-handler; documented deadlock avoidance in `log_session_manager.go` (async stream start).

### `internal/processmanager/`

| Mutex | File | Acquired by | Re-entrancy risk |
|-------|------|-------------|------------------|
| `quiesceMu` (`sync.RWMutex`, package) | `quiesce.go` | `SetQuiesced`, `IsQuiesced` | Monitor loop checks without holding other PM locks |
| `controlPlanePullOnRecreateMu` | `controlplane_reconcile.go` | CP pull-on-recreate guard | Short-lived; no nested PM mutex |
| `localLaunchLocks` (`sync.Map` of per-UUID `*sync.Mutex`) | `local_launch.go` | `withLocalLaunchLock` | Per-UUID; no global ordering with volumemount |
| `task_queue.mu` | `task_queue.go` | Task enqueue/dequeue | Worker processes tasks; `CleanupMicroserviceVolumes` called after container remove, not under queue lock |
| `restart_checker.mu` | `restart_checker.go` | Restart policy state | Independent |
| `collectLogTailHandler.mu` | `manager.go` | Log collection callback | Per-handler |

**Monitor loop:** `containersMonitor` has top-level `recover()`. Remove callbacks invoke `volumemount.CleanupMicroserviceVolumes` without holding PM mutexes — **LC-3 safe** after 19-C.

## Fixes applied (19-A, 19-C, 19-E)

### 19-A — LC-1 `indexLock` re-entrancy

- Added `syncMicroserviceSymlinksUnsafe`; public `syncMicroserviceSymlinks` locks and delegates.
- `updateVolumeMount` calls Unsafe while `ProcessVolumeMountChanges` holds `indexLock`.
- `ProcessVolumeMountChanges` uses `rebuildTypeCacheUnsafe` (existing).
- Test: `TestUpdateVolumeMount_NoDeadlockWhenIndexLockHeld`.

### 19-C — LC-3 deprovision vs PM cleanup

- `Clear()` / `ClearControllerArtifacts()`: index reset under `indexLock`, then release before `clearWalk` / `RemoveAll`.
- `CleanupMicroserviceVolumes()`: `cleanupMicroserviceVolumesIndexUnsafe` under brief lock; `RemoveAll` outside lock.
- Test: `TestClear_ConcurrentCleanupMicroserviceVolumes_NoDeadlock`.

### 19-E — verification only

- Full mutex inventory and tooling runs documented below.
- No new code fixes required beyond 19-A/19-C.

## Tooling results

Run on 2026-06-16 (darwin, Go toolchain from CI).

| Command | Result |
|---------|--------|
| `make pre-it` | **PASS** — vet, golangci-lint, unit tests (`volumemount`, `fieldagent`, `processmanager`) |
| `make test-deadlock` | **PASS** — `-tags deadlock` (same packages; see note below) |
| `make test-lifecycle-race` | **PASS** (2026-06-16) — fast subset (`volumemount`, `fieldagent`, `processmanager`) |
| `make test-race` | Full unit-test tree (`./cmd`, `./internal`, `./pkg`, `./test`); `-short`; host-native |
| `make test-linux-race` | Same as `test-race` inside Linux Docker (all three build-tag passes; use on macOS) |

### `test-deadlock` note

`github.com/sasha-s/go-deadlock` is listed in `go.mod` (indirect) but there is no `//go:build deadlock` swap of `sync.Mutex` to go-deadlock detectors yet. The Makefile target runs tests with `-tags deadlock`; LC-1/LC-3 coverage relies on timeout-based concurrency tests in `manager_test.go`. Adding a `deadlock` build-tag mutex shim remains a follow-up (DL-T).

### `test-lifecycle-race` (fixed in 19-E follow-up)

| Package | Fix |
|---------|-----|
| `fieldagent` | `getAPIClient()` / `setAPIClient()` / `replaceAPIClient()` under `fa.mu`; all controller HTTP paths use `getAPIClient()`; daemon `Provision()` no longer calls async `Update()` (sync client refresh already done) |
| `processmanager` | `captureEvents` test helper mutex around concurrent `append` during parallel drain workers |

## Deferred risks

| Risk | Severity | Notes |
|------|----------|-------|
| `Update()` vs `Provision()` `apiClient` race | **Fixed** — synchronized client access; removed redundant `Update()` from daemon `Provision()` |
| `test-lifecycle-race` test helpers | **Fixed** — `captureEvents` mutex |
| go-deadlock build tag | Low | Would catch future LC-1 regressions at test time |
| `postStatusWorker` / other FA workers without `recover()` | Low | Panic kills one worker goroutine; daemon continues (see worker table) |
| `pruning` workers (`internal/pruning/`) | Low | Out of DL-1 scope; no `recover()` on threshold/frequency workers |
| `healthcheck.Runner.run` | Low | Out of DL-1 scope; no `recover()` |
| Provision during background deprovision | Operational | `provisioningMu` unlocked after `Deprovision` returns while cleanup runs; by design for fast HTTP response |

## Worker recover status

| Worker | File | `recover()`? |
|--------|------|--------------|
| `runChangesWorker` | `workers.go` | Yes — worker + per-tick + `processChanges` goroutine (19-D) |
| `pingControllerWorker` | `workers.go` | Yes — top-level defer |
| `postStatusWorker` | `workers.go` | **No** — panic exits worker |
| `localAPITokenRotationWorker` | `workers.go` | No |
| `upgradeScanWorker` | `workers.go` | No |
| `serviceAccountTokenRotationWorker` | `workers.go` | No |
| `controllerRegisterWorker` | `controller_register.go` | No |
| `containersMonitor` | `manager.go` | Yes — top-level defer |
| Deprovision background goroutine | `agent.go` | Yes — partial (stop MS, module notify, SQLite clear) |
| `thresholdPruningWorker` / `frequencyPruningWorker` | `pruning/manager.go` | No (out of scope) |
| `healthcheck.Runner.run` | `healthcheck/runner.go` | No (optional, out of scope) |

## Recommendations for future work

1. **go-deadlock shim:** `internal/syncutil/deadlock.go` with `//go:build deadlock` re-exporting go-deadlock `RWMutex`/`Mutex`; use in `VolumeMountManager` tests or production structs under audit.
2. **FA worker hardening:** Add `recover()` + stack log to `postStatusWorker` and rotation workers (mirror `pingControllerWorker`).
3. **Provision race:** Serialize `apiClient` access in `Provision` + `Update` async path (19-B follow-up).
4. **Race CI:** Dedicated optional job after test fixes; exclude from default `pre-it`.
5. **Lock docs:** Keep `*Unsafe` precondition comments when adding new volumemount index helpers (DL-2).
