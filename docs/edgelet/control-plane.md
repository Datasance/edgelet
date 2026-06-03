# Edgelet ControlPlane (operator guide)

> **Status:** Done (Plan 12, 2026-06-04) — spec: `.cursor/edgelet/docs/12-control-plane-controller.md`

Edgelet can host **one** Datasance Controller container per node when you apply a `kind: ControlPlane` manifest. There is **no** default controller on install.

## Fixtures

| Path | `metadata.namespace` | `metadata.name` | Use |
|------|----------------------|-------------------|-----|
| `test/deployment-yamls/controlplane.yaml` | `bar` | `foo` | Dev smoke / manual 12-3 |
| IT fixture (12-8) | `default` | `pot` | `test/control-plane/fixtures/controlplane-it.yaml` — DNS: `edgelet.controller…`, `controller.default…`, `default.pot…` |

## Deploy

```bash
edgelet deploy -f controlplane.yaml
edgelet deploy -f controlplane.yaml --timeout=20m   # optional: override default poll budget
```

- `apiVersion: edgelet.iofog.org/v1`
- `kind: ControlPlane`
- Re-apply updates image/env (same controller UUID). To change `metadata.name` or `metadata.namespace`, delete first.

### Long deploys (Plan 12-9)

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

This is the **only** supported way to remove the controller deployment. `edgelet ms rm` on the controller UUID is rejected; Edgelet will reconcile the container back if the ControlPlane record still exists.

## Ports

| Host | Container | Service |
|------|-----------|---------|
| 51121 | `spec.controller.port` (default 51121) | Controller API |
| 80 | `spec.ecnViewerPort` (default 8008) | ECN viewer |

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

## Image load (global CLI, Plan 12-9)

Large archives use async load (same pattern as `edgelet image pull`):

```bash
edgelet image load /path/to/archive.tar
```

Default poll budget **30 minutes**; use `edgelet --timeout` to override.

## Full contract

See `.cursor/edgelet/CONTROL-PLANE.md` in the Edgelet repository. Production CLI rules: RFC §25 in `.cursor/edgelet/docs/00-rfc.md`.
