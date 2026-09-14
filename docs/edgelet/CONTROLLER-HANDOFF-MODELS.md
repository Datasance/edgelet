# Controller handoff — registries, models, and microservice catalog

This document is for the **Pot / Datasance Controller** team. It is the agent-side contract for registry extras, fleet models, microservice catalog + container fields, `getChanges` / `GET models`, and fog status. Existing Pot path prefixes stay **`/api/v3/…`**. Additive JSON only — do not rename existing keys or routes.

Operator YAML: [manifest-reference.md](manifest-reference.md) · [models.md](models.md). Edgelet local API stays **`/v1/…`**.

The agent already consumes every shape below. Controller CRUD and UI can implement against this page without reading internal specs.

---

## Identity and paths

| Item | Contract |
|------|----------|
| Model identity | **`uuid`** (required string) + unique **`name`** (DNS-1123). No integer `id` |
| On-disk name | One directory per `name`: `{diskDirectory}/models/{name}/` |
| Bind items | **`name` only** — never send uuid or a host content path in `models.items[]` |
| `GET models` | `{controllerUrl}/agent/models` (same style as `registries`, `microservices`) |
| `getChanges` flag | **`models`** (boolean). On `true`, and on initialization, the agent reloads the list |
| Older controllers | If the `models` flag is absent, the agent still `PUT status` with `modelStatus: "[]"` and `activeModels: 0` — never fail status |

While a node is provisioned, a **managed** model wins that `name` (pull spec + on-disk tree). Local `kind: Model` apply for a managed name is rejected. Controller microservices bind **managed** names only; local microservices bind **local** names only.

---

## Registry object (existing payload, extras)

`type` defaults to `"oci"` when omitted.

```json
{
  "id": 5,
  "url": "https://huggingface.co",
  "isPublic": false,
  "userName": "",
  "password": "hf_…",
  "userEmail": "",
  "type": "hf",
  "ca": "",
  "insecure": false
}
```

| Field | Type | Notes |
|-------|------|--------|
| `type` | string | `"oci"` (default) or `"hf"` |
| `ca` | string | Optional base64 PEM CA bundle |
| `insecure` | boolean | Default `false`; `true` allows `http://` and skips TLS verify |

Existing **`registries: true`** on `getChanges` reloads these extended rows. Image pull on the agent still requires `type: oci`.

---

## Model object

```json
{
  "uuid": "3f2c8a1e-2b64-4c0d-9f11-0a1b2c3d4e5f",
  "name": "test-model",
  "repo": "second-state/Llama-2-7B-Chat-GGUF",
  "revision": "064fe43ea8c1e1f93477ef4a170bdc2b244ef02c",
  "registryId": 5,
  "files": ["llama-2-7b-chat.Q5_K_M.gguf"],
  "format": "gguf"
}
```

| Field | Type | Notes |
|-------|------|--------|
| `uuid` | string | **Required.** Controller primary key. No integer `id` |
| `name` | string | **Required.** Unique in the controller. DNS-1123; matches Edgelet `metadata.name` |
| `repo` | string | Upstream path **without** host |
| `revision` | string | Pin; empty means `latest` (OCI) or `main` (HF) |
| `registryId` | integer | Registry row id; `type` must match the pull adapter |
| `files` | string[] | HF only; ignored for OCI |
| `format` | string | Optional hint: `gguf`, `safetensors`, `onnx`, `pytorch`, `tensorrt`, `unknown` |

Unknown extra keys on the model object are ignored.

### `GET /api/v3/agent/models`

Response envelope:

```json
{
  "models": [
    {
      "uuid": "3f2c8a1e-2b64-4c0d-9f11-0a1b2c3d4e5f",
      "name": "test-model",
      "repo": "second-state/Llama-2-7B-Chat-GGUF",
      "revision": "064fe43ea8c1e1f93477ef4a170bdc2b244ef02c",
      "registryId": 5,
      "files": ["llama-2-7b-chat.Q5_K_M.gguf"],
      "format": "gguf"
    }
  ]
}
```

Rows missing `uuid` or `name` are skipped. A controller with no models route is treated as an empty list (agent continues).

### `getChanges` `models`

| Flag | Agent behavior |
|------|----------------|
| `models: true` (or initialization) | `GET models` → replace-all `controller_models` → upsert/pull |
| `models: false` or omitted | No reload. Status still includes the additive fog keys below |
| `registries: true` | Reload registry rows including `type`, `ca`, `insecure` |

---

## Microservice extras

Additive keys on the existing microservice object. Unknown keys are ignored.

### Catalog

```json
"models": {
  "bindPath": "/models",
  "permissions": "ro",
  "items": [{ "name": "test-model" }, { "name": "qwen3-8-27b" }]
}
```

| Field | Type | Notes |
|-------|------|--------|
| `bindPath` | string | Required when `items` is non-empty. Absolute **container** path |
| `permissions` | string | `ro` (default) or `rw`. Catalog-level only |
| `items[].name` | string | DNS-1123. Duplicate names are a validate error |

Container path is always **`{bindPath}/{name}/`** = that model's Ready `content/`. One bind of a per-microservice projection. No per-item permissions. No host content path. No auto-injected model env vars.

Local MS → local models only. Controller MS → managed models only.

### Process argv

| Field | Notes |
|-------|--------|
| `entrypoint` | Array or omit. Empty / `[]` / omit = image default (agent does not send empty argv) |
| `commands` | Preferred argv after entrypoint. Empty / `[]` / omit = image default |
| `cmd` | **Alias of `commands`**. If both are present, **`commands` wins**. Keep `cmd` forever |

There is no per-microservice `stopSignal`.

### Container fields (agent-applied)

| Field | Type | Units / notes |
|-------|------|----------------|
| `runAsGroup` | string | Separate from `runAsUser` |
| `readOnlyRootFilesystem` | boolean | No auto-inject of `/tmp` |
| `workingDir` | string | Absolute container path |
| `cpus` | number | Docker `--cpus` (float count), not node `cpuLimit` percent |
| `memoryLimit` | integer | **MiB** |
| `memoryReservation` | integer | **MiB**; allowed without `memoryLimit` |
| `memorySwap` | integer | **`-1`** unlimited; else MiB **memory+swap total**. Requires `memoryLimit` unless `-1` |
| `shmSize` | integer | `/dev/shm` in **MiB** |
| `sysctls` | object | String map; safe-sysctl allowlist |
| `ulimits` | object | Map of `{soft, hard}` only |
| `devices` | array | `{hostPath, containerPath, permissions}` |
| `tmpfs` | array | `{containerPath, size?, mode?}`. `size` is MiB |
| `healthCheck` | object | Times in **seconds** |
| `annotations` | string or object | Applied |

---

## Validation rules (copy onto Pot)

The agent still enforces these. Implement the same checks on create/update so operators see errors before sync.

### Catalog

- Empty or omitted `items` → `bindPath` not required.
- Non-empty `items` → `bindPath` required and must be an absolute container path.
- `permissions` omitted → `ro`. Only `ro` or `rw`.
- Duplicate `items[].name` → error.
- `bindPath` or `{bindPath}/{name}` colliding with a volume `containerDestination` or `tmpfs.containerPath` → error.
- Controller MS item naming a local-only model → error (agent: microservice **FAILED**).
- Local MS item naming a managed model → error.

### Start gate (agent runtime)

- Do not start until every named item is **Ready**.
- Pending/Pulling → persist; MS **QUEUED**; status explains wait for download.
- Unknown or Failed name on a controller MS → MS **FAILED** + status text (include model `lastError` when present).
- Item add/remove/re-pull → in-place projection, **no** container recreate.
- Recreate when `bindPath` or catalog `permissions` changes, or on `rebuild` / other spec drift.

### `memorySwap`

- Omitted → no swap constraint from this field.
- `-1` → unlimited; `memoryLimit` not required.
- Any other value must be a positive MiB **memory+swap total** and **requires `memoryLimit`**.

### `runAsUser` / `runAsGroup`

- `runAsUser` containing `:` **and** `runAsGroup` set → error.

### Sysctls

Allowlist (Kubernetes safe set): `kernel.shm_rmid_forced`, `net.ipv4.ip_local_port_range`, `net.ipv4.tcp_syncookies`, `net.ipv4.ping_group_range`, `net.ipv4.ip_unprivileged_port_start`, `net.ipv4.ip_local_reserved_ports`, `net.ipv4.tcp_keepalive_time`, `net.ipv4.tcp_fin_timeout`, `net.ipv4.tcp_keepalive_intvl`, `net.ipv4.tcp_keepalive_probes`, `net.ipv4.tcp_rmem`, `net.ipv4.tcp_wmem`, `net.ipv4.tcp_slow_start_after_idle`, `net.ipv4.tcp_notsent_lowat`.

- Unknown key → error.
- `hostNetworkMode: true` → reject `net.*`.
- `ipcMode: host` → reject IPC-namespaced names (`kernel.shm*`, `kernel.msg*`, `kernel.sem*`, `fs.mqueue.*`).
- `pidMode: host` does not change sysctl validation.

### Ulimits

Allowlist: `core`, `cpu`, `data`, `fsize`, `locks`, `memlock`, `msgqueue`, `nice`, `nofile`, `nproc`, `rss`, `rtprio`, `rttime`, `sigpending`, `stack`.

- Must be `{soft, hard}` objects. Scalar values → error.
- Unknown key → error.
- `-1` = unlimited. Unlimited soft requires unlimited hard.
- If neither side is `-1`, `soft` must be `<= hard`.
- Nested `ulimits.cpu` is RLIMIT_CPU (seconds), not `cpus`.

### Devices

- `hostPath` required, absolute, and must be under **`/dev`**.
- `containerPath` required.
- `permissions` is Docker-style `r` / `w` / `m` (any combination). Empty → `rwm`.

### `tmpfs`

- `containerPath` required and absolute.
- `size` when set must be `> 0` (MiB).

### `workingDir`

- When set, must be an absolute container path.

### `cpus` / `memoryReservation` / `shmSize`

- When set, must be `> 0`.

---

## Fog status (`PUT` status, additive)

**Managed models only.** Local models never appear.

| Field | Type | Notes |
|-------|------|--------|
| `modelStatus` | string | JSON **string** (not a raw array) of status items |
| `activeModels` | integer | Count of managed models in the last replace-all list |
| `modelLastUpdate` | integer | Unix seconds; `0` when the list is empty |

Older controllers ignore these keys. When the controller has no models (or no `models` flag), send `modelStatus: "[]"`, `activeModels: 0`, `modelLastUpdate: 0`.

`modelStatus` item (after `JSON.parse`):

```json
{
  "uuid": "3f2c8a1e-2b64-4c0d-9f11-0a1b2c3d4e5f",
  "name": "test-model",
  "state": "Ready",
  "digest": "sha256:…",
  "resolvedRevision": "064fe43…",
  "revisionFloating": false,
  "totalBytes": 2840000000,
  "lastError": ""
}
```

| Field | Meaning |
|-------|---------|
| `state` | `Pending`, `Pulling`, `Ready`, `Failed` |
| `digest` | OCI manifest digest after pull |
| `resolvedRevision` | HF commit (or resolved OCI ref) |
| `revisionFloating` | `true` when the requested revision is a floating tag/branch |
| `lastError` | Pull/reconcile error text; empty when none |

---

## Controller work remaining (CRUD / UI)

The agent consume path is implemented. Remaining controller work:

- [ ] Registry REST: persist and return `type`, `ca`, `insecure`
- [ ] Model CRUD (create / read / update / delete) with **`uuid` + unique `name`**
- [ ] `getChanges` flag `models` and `GET /api/v3/agent/models` → `{ "models": [ … ] }`
- [ ] Microservice REST: catalog + container fields; accept `cmd` and `permissions`; prefer `commands`
- [ ] Copy the validation tables above onto create/update
- [ ] Dashboards: parse fog `modelStatus` / `activeModels` / `modelLastUpdate`

---

## Engine note

`edgelet` and `docker` apply every new container field. Podman reuses the Docker HostConfig mapping; `cdiDevices` is not wired on Podman. See [container-engine.md](container-engine.md#podman-field-coverage).
