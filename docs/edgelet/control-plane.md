# Edgelet ControlPlane (operator guide)

Edgelet can host **one** Datasance Controller container per node when you apply a `kind: ControlPlane` manifest. There is **no** default controller on install.

## Core rules

- At most **one** controller deployment per Edgelet (SQLite singleton).
- Edgelet may run with `controllerUrl` pointing to a **remote** cluster; local ControlPlane is optional.
- Reconcile runs **before** managed microservices; accidental `docker rm` recreates the container while the DB row exists.
- Only `edgelet controlplane delete` (or `DELETE /v1/system/controlplane`) removes the deployment and cleans volumes.

## Fixtures

| Path | `metadata.namespace` | `metadata.name` | Use |
|------|----------------------|-------------------|-----|
| `test/deployment-yamls/controlplane.yaml` | `bar` | `foo` | Dev smoke / manual apply |
| IT fixture | `default` | `pot` | `test/control-plane/fixtures/controlplane-it.yaml` — DNS: `edgelet.controller…`, `controller.default…`, `default.pot…` |

## Deploy

```bash
edgelet deploy -f controlplane.yaml
edgelet deploy -f controlplane.yaml --timeout=20m   # optional: override default poll budget
```

- `apiVersion: edgelet.iofog.org/v1`
- `kind: ControlPlane`
- Re-apply updates image/env (same controller UUID). To change `metadata.name` or `metadata.namespace`, delete first.

### Long deploys (async apply)

Control plane apply is **asynchronous** (Kubernetes-style): the CLI returns **202** immediately, polls apply status, then confirms `edgelet controlplane get` shows `runtimeState: running`.

| Default | Value |
|---------|-------|
| Poll budget (CP deploy) | **15 minutes** |
| Per status request | **60 seconds** |
| Override | `edgelet --timeout` on poll total |

If the CLI times out, work may still continue on the daemon. Check:

```bash
edgelet controlplane get
# or poll: GET /v1/deploy/controlplane:apply/<operationId>
```

Do **not** run a second deploy immediately after a timeout (avoids duplicate apply). Registry manifests remain **synchronous** (fast).

## Inspect

```bash
edgelet controlplane get
edgelet controlplane get --manifest   # secrets masked
edgelet ms ls --source controlplane
edgelet ms inspect <uuid|namespace.name>   # engine container inspect (raw.engineInspect)
```

Use **controlplane get** for deployment status and manifest; **ms inspect** returns the runtime container inspect (same as other microservices), not the SQLite manifest row.

## Delete

```bash
edgelet controlplane delete
```

This is the **only** supported way to remove the controller deployment while the agent is **unprovisioned**. `edgelet ms rm` on the controller UUID is rejected when provisioned; Edgelet will reconcile the container back if the ControlPlane record still exists.

While the agent is **provisioned**, `edgelet controlplane delete` is also rejected — deprovision the agent first.

## Controller registration (system fog)

After the local ControlPlane container is running and the agent is provisioned, Edgelet registers the controller workload once with Pot (`POST /api/v3/agent/controller/register`). Pot then lists the controller microservice with `isController: true`.

**Operator requirement:** the system fog must be created on Controller with **`isSystem: true`**. Registration uses the agent fog token and fails with **403** on non-system fogs.

Spec updates from Pot (`microserviceList`) merge into the local ControlPlane deployment; the process manager does not ADD/DELETE a duplicate controller container.

## Ports

| Host | Container | Service |
|------|-----------|---------|
| 51121 | `spec.controller.port` (default 51121) | Controller API |
| 80 | `spec.console.port` (default 8008) | EdgeOps Console |

## DNS (embedded)

Controller microservice identity matches other workloads: **application** = `metadata.namespace`, **name** = `metadata.name`.

From workloads on the bridge network, three names resolve to the controller IP:

1. `edgelet.controller.svc.bridge.local` — stable alias  
2. `controller.<metadata.namespace>.svc.bridge.local` — namespace alias  
3. `<metadata.namespace>.<metadata.name>.svc.bridge.local` — standard `appName.microserviceName` pattern  

(`edgelet ms ls --source controlplane` shows the same application and name fields.)

Set `controllerUrl` in Edgelet config separately (Edgelet does not auto-update it on deploy).

## Router CA (siteCA / localCA)

Not in the Edgelet `ControlPlane` manifest. Use **potctl/iofogctl** to import site/local CA material via the Controller REST API after the controller is up.

## Watchdog

Edgelet does **not** delete the controller container when labels match the DB row (`edgelet.iofog.org/system=true`, `role=controller`, UUID, container ID, image ref). Manual `docker run` without matching identity may be removed by the watchdog.

## Image load (global CLI)

Large archives use async load (same pattern as `edgelet image pull`):

```bash
edgelet image load /path/to/archive.tar
```

Default poll budget **30 minutes**; use `edgelet --timeout` to override.

## Integration tests

```bash
./test/control-plane/run-all.sh
```

See [test/control-plane/README.md](../../test/control-plane/README.md).
